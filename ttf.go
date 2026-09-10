package pdfwriter

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf16"
)

// ttfFont holds the parts of a TrueType file that PDF needs: a character map,
// advance widths, and the raw bytes to embed.
type ttfFont struct {
	data       []byte
	unitsPerEm float64
	numGlyphs  int
	advances   []uint16 // per glyph, in font design units
	cmap       map[rune]uint16
	bbox       [4]int16 // xMin, yMin, xMax, yMax
	ascent     int16
	descent    int16
	capHeight  int16
	italic     bool
	fixedPitch bool

	// used records every glyph actually drawn, so only those need widths
	// and reverse mappings in the output.
	used map[uint16]rune
}

// glyph looks up the glyph index for a rune.
func (t *ttfFont) glyph(r rune) (uint16, bool) {
	gid, ok := t.cmap[r]
	return gid, ok
}

// advance returns a glyph's advance width scaled to glyph space, where the em
// is 1000 units regardless of the font's own resolution.
func (t *ttfFont) advance(gid uint16) float64 {
	if int(gid) >= len(t.advances) {
		return 0
	}
	return float64(t.advances[gid]) * 1000.0 / t.unitsPerEm
}

// scale converts a value in font design units to glyph space.
func (t *ttfFont) scale(v int16) int {
	return int(float64(v) * 1000.0 / t.unitsPerEm)
}

// AddTTFFont reads a TrueType font from disk and returns a handle for
// [Doc.SetFont]. The name is only used inside the PDF, so any short
// identifier will do.
//
// The whole font file is embedded, which keeps this library simple at the cost
// of file size: expect the PDF to grow by roughly the compressed size of the
// font. Fonts with PostScript outlines, which usually have an .otf extension,
// are not supported.
func (d *Doc) AddTTFFont(name, path string) (*Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pdfwriter: reading font: %w", err)
	}
	return d.AddTTFFontData(name, data)
}

// AddTTFFontData is [Doc.AddTTFFont] for a font already in memory, which suits
// fonts included with go:embed.
func (d *Doc) AddTTFFontData(name string, data []byte) (*Font, error) {
	clean := sanitiseName(name)
	if _, taken := d.fontIdx[clean]; taken {
		return nil, fmt.Errorf("pdfwriter: font name %q is already in use", clean)
	}
	t, err := parseTTF(data)
	if err != nil {
		return nil, err
	}
	f := &Font{name: clean, ttf: t}
	d.register(f)
	return f, nil
}

// sanitiseName reduces a name to characters that need no escaping when written
// as a PDF name object.
func sanitiseName(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		}
	}
	if sb.Len() == 0 {
		return "EmbeddedFont"
	}
	return sb.String()
}

// reader reads the big-endian values that make up an sfnt font file.
type reader struct {
	b []byte
}

func (r reader) u16(off int) (uint16, error) {
	if off < 0 || off+2 > len(r.b) {
		return 0, fmt.Errorf("pdfwriter: font truncated at %d", off)
	}
	return binary.BigEndian.Uint16(r.b[off:]), nil
}

func (r reader) i16(off int) (int16, error) {
	v, err := r.u16(off)
	return int16(v), err
}

func (r reader) u32(off int) (uint32, error) {
	if off < 0 || off+4 > len(r.b) {
		return 0, fmt.Errorf("pdfwriter: font truncated at %d", off)
	}
	return binary.BigEndian.Uint32(r.b[off:]), nil
}

// parseTTF extracts everything needed to embed and lay out a TrueType font.
func parseTTF(data []byte) (*ttfFont, error) {
	r := reader{data}
	version, err := r.u32(0)
	if err != nil {
		return nil, fmt.Errorf("pdfwriter: not a font file: %w", err)
	}
	switch version {
	case 0x00010000, 0x74727565: // 'true'
	case 0x4F54544F: // 'OTTO'
		return nil, fmt.Errorf(
			"pdfwriter: this font has PostScript outlines, which are not supported; use a TrueType (.ttf) font")
	case 0x74746366: // 'ttcf'
		return nil, fmt.Errorf(
			"pdfwriter: TrueType collections (.ttc) are not supported; extract a single font first")
	default:
		return nil, fmt.Errorf("pdfwriter: unrecognised font format %#08x", version)
	}

	numTables, err := r.u16(4)
	if err != nil {
		return nil, err
	}
	tables := make(map[string][]byte, numTables)
	for i := 0; i < int(numTables); i++ {
		rec := 12 + i*16
		if rec+16 > len(data) {
			return nil, fmt.Errorf("pdfwriter: font table directory truncated")
		}
		tag := string(data[rec : rec+4])
		off, err := r.u32(rec + 8)
		if err != nil {
			return nil, err
		}
		length, err := r.u32(rec + 12)
		if err != nil {
			return nil, err
		}
		end := uint64(off) + uint64(length)
		if end > uint64(len(data)) {
			// Some fonts pad the final table short; clamp rather than
			// reject an otherwise usable file.
			end = uint64(len(data))
		}
		if uint64(off) > end {
			return nil, fmt.Errorf("pdfwriter: font table %q out of range", tag)
		}
		tables[tag] = data[off:end]
	}

	for _, required := range []string{"head", "hhea", "maxp", "hmtx", "cmap", "glyf", "loca"} {
		if _, ok := tables[required]; !ok {
			return nil, fmt.Errorf("pdfwriter: font is missing the %q table", required)
		}
	}

	t := &ttfFont{data: data, used: make(map[uint16]rune)}

	head := reader{tables["head"]}
	upem, err := head.u16(18)
	if err != nil {
		return nil, err
	}
	if upem == 0 {
		return nil, fmt.Errorf("pdfwriter: font declares unitsPerEm of zero")
	}
	t.unitsPerEm = float64(upem)
	for i, off := range []int{36, 38, 40, 42} {
		if t.bbox[i], err = head.i16(off); err != nil {
			return nil, err
		}
	}

	maxp := reader{tables["maxp"]}
	ng, err := maxp.u16(4)
	if err != nil {
		return nil, err
	}
	t.numGlyphs = int(ng)

	hhea := reader{tables["hhea"]}
	if t.ascent, err = hhea.i16(4); err != nil {
		return nil, err
	}
	if t.descent, err = hhea.i16(6); err != nil {
		return nil, err
	}
	numHMetrics, err := hhea.u16(34)
	if err != nil {
		return nil, err
	}
	if numHMetrics == 0 {
		return nil, fmt.Errorf("pdfwriter: font declares no horizontal metrics")
	}

	if err := t.parseHmtx(tables["hmtx"], int(numHMetrics)); err != nil {
		return nil, err
	}
	if err := t.parseCmap(tables["cmap"]); err != nil {
		return nil, err
	}

	// OS/2 and post are optional; their absence only costs metadata quality.
	if os2, ok := tables["OS/2"]; ok {
		o := reader{os2}
		if v, err := o.u16(0); err == nil && v >= 1 {
			if ch, err := o.i16(88); err == nil {
				t.capHeight = ch
			}
		}
	}
	if t.capHeight <= 0 {
		t.capHeight = int16(float64(t.ascent) * 0.7)
	}
	if post, ok := tables["post"]; ok {
		p := reader{post}
		if angle, err := p.u32(4); err == nil {
			t.italic = angle != 0
		}
		if fixed, err := p.u32(16); err == nil {
			t.fixedPitch = fixed != 0
		}
	}
	return t, nil
}

// parseHmtx reads advance widths. Only the first numHMetrics glyphs carry one;
// any remaining glyphs repeat the last value, which is how monospaced tails
// and CJK fonts stay compact.
func (t *ttfFont) parseHmtx(b []byte, numHMetrics int) error {
	r := reader{b}
	if numHMetrics > t.numGlyphs {
		numHMetrics = t.numGlyphs
	}
	t.advances = make([]uint16, t.numGlyphs)
	last := uint16(0)
	for i := 0; i < numHMetrics; i++ {
		w, err := r.u16(i * 4)
		if err != nil {
			return err
		}
		t.advances[i] = w
		last = w
	}
	for i := numHMetrics; i < t.numGlyphs; i++ {
		t.advances[i] = last
	}
	return nil
}

// maxCmapEntries caps how many characters are mapped, guarding against a
// malformed table that claims an enormous range.
const maxCmapEntries = 1 << 20

// parseCmap builds the rune-to-glyph map, preferring a full Unicode subtable
// over a Basic-Multilingual-Plane-only one.
func (t *ttfFont) parseCmap(b []byte) error {
	r := reader{b}
	numSubtables, err := r.u16(2)
	if err != nil {
		return err
	}

	// Higher rank wins. Format 12 subtables reach beyond U+FFFF, so they
	// are preferred where a font offers both.
	best, bestRank := -1, -1
	for i := 0; i < int(numSubtables); i++ {
		rec := 4 + i*8
		platform, err := r.u16(rec)
		if err != nil {
			return err
		}
		encoding, err := r.u16(rec + 2)
		if err != nil {
			return err
		}
		offset, err := r.u32(rec + 4)
		if err != nil {
			return err
		}
		if int(offset) >= len(b) {
			continue
		}
		format, err := r.u16(int(offset))
		if err != nil {
			continue
		}

		rank := -1
		switch {
		case platform == 3 && encoding == 10 && format == 12:
			rank = 4 // Windows, full Unicode
		case platform == 0 && format == 12:
			rank = 3 // Unicode platform, full repertoire
		case platform == 3 && encoding == 1 && format == 4:
			rank = 2 // Windows, BMP only
		case platform == 0 && format == 4:
			rank = 1 // Unicode platform, BMP only
		}
		if rank > bestRank {
			best, bestRank = int(offset), rank
		}
	}
	if best < 0 {
		return fmt.Errorf("pdfwriter: font has no usable Unicode character map")
	}

	t.cmap = make(map[rune]uint16)
	format, err := r.u16(best)
	if err != nil {
		return err
	}
	switch format {
	case 4:
		return t.parseCmap4(b, best)
	case 12:
		return t.parseCmap12(b, best)
	default:
		return fmt.Errorf("pdfwriter: unsupported character map format %d", format)
	}
}

// parseCmap4 reads the segmented BMP format.
func (t *ttfFont) parseCmap4(b []byte, off int) error {
	r := reader{b}
	segCountX2, err := r.u16(off + 6)
	if err != nil {
		return err
	}
	segCount := int(segCountX2) / 2
	endBase := off + 14
	startBase := endBase + segCount*2 + 2
	deltaBase := startBase + segCount*2
	rangeBase := deltaBase + segCount*2

	for seg := 0; seg < segCount; seg++ {
		end, err := r.u16(endBase + seg*2)
		if err != nil {
			return err
		}
		start, err := r.u16(startBase + seg*2)
		if err != nil {
			return err
		}
		delta, err := r.i16(deltaBase + seg*2)
		if err != nil {
			return err
		}
		rangeOffAddr := rangeBase + seg*2
		rangeOff, err := r.u16(rangeOffAddr)
		if err != nil {
			return err
		}
		if start > end {
			continue
		}
		for c := uint32(start); c <= uint32(end); c++ {
			if c == 0xFFFF || len(t.cmap) >= maxCmapEntries {
				continue
			}
			var gid uint16
			if rangeOff == 0 {
				gid = uint16(int32(c) + int32(delta))
			} else {
				// The offset is measured from the address of the
				// idRangeOffset entry itself.
				addr := rangeOffAddr + int(rangeOff) + 2*int(c-uint32(start))
				g, err := r.u16(addr)
				if err != nil || g == 0 {
					continue
				}
				gid = uint16(int32(g) + int32(delta))
			}
			if gid != 0 && int(gid) < t.numGlyphs {
				t.cmap[rune(c)] = gid
			}
		}
	}
	return nil
}

// parseCmap12 reads the grouped full-Unicode format.
func (t *ttfFont) parseCmap12(b []byte, off int) error {
	r := reader{b}
	nGroups, err := r.u32(off + 12)
	if err != nil {
		return err
	}
	for i := uint32(0); i < nGroups; i++ {
		rec := off + 16 + int(i)*12
		start, err := r.u32(rec)
		if err != nil {
			return err
		}
		end, err := r.u32(rec + 4)
		if err != nil {
			return err
		}
		startGID, err := r.u32(rec + 8)
		if err != nil {
			return err
		}
		if start > end || end > 0x10FFFF {
			continue
		}
		for c := start; c <= end; c++ {
			if len(t.cmap) >= maxCmapEntries {
				return nil
			}
			gid := startGID + (c - start)
			if gid != 0 && gid < uint32(t.numGlyphs) {
				t.cmap[rune(c)] = uint16(gid)
			}
		}
	}
	return nil
}

// writeTTFFont emits the five objects that make up an embedded TrueType font
// and returns the object number of the top-level font dictionary.
//
// The font is written as a Type0 composite font with Identity-H encoding, so
// text is addressed by glyph index rather than character code. That is what
// lifts the 256-character limit of the built-in fonts.
func (d *Doc) writeTTFFont(objs *objects, f *Font) (int, error) {
	t := f.ttf

	fontDict := objs.reserve()
	cidFont := objs.reserve()
	descriptor := objs.reserve()
	fontFile := objs.reserve()
	toUnicode := objs.reserve()

	// Embed the font file itself, compressed. Length1 records the original
	// size, which viewers need in order to decompress it.
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(t.data); err != nil {
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	var stream bytes.Buffer
	fmt.Fprintf(&stream,
		"<< /Length %d /Length1 %d /Filter /FlateDecode >>\nstream\n",
		zbuf.Len(), len(t.data))
	stream.Write(zbuf.Bytes())
	stream.WriteString("\nendstream")
	objs.setRaw(fontFile, stream.Bytes())

	flags := 32 // nonsymbolic
	if t.fixedPitch {
		flags |= 1
	}
	if t.italic {
		flags |= 64
	}
	objs.set(descriptor, fmt.Sprintf(
		"<< /Type /FontDescriptor /FontName /%s /Flags %d "+
			"/FontBBox [%d %d %d %d] /ItalicAngle 0 /Ascent %d /Descent %d "+
			"/CapHeight %d /StemV 80 /FontFile2 %d 0 R >>",
		f.name, flags,
		t.scale(t.bbox[0]), t.scale(t.bbox[1]), t.scale(t.bbox[2]), t.scale(t.bbox[3]),
		t.scale(t.ascent), t.scale(t.descent), t.scale(t.capHeight),
		fontFile))

	objs.set(cidFont, fmt.Sprintf(
		"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /%s "+
			"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> "+
			"/FontDescriptor %d 0 R /DW 1000 /W %s /CIDToGIDMap /Identity >>",
		f.name, descriptor, t.widthArray()))

	objs.set(fontDict, fmt.Sprintf(
		"<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H "+
			"/DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>",
		f.name, cidFont, toUnicode))

	cmapStream, err := d.stream([]byte(t.toUnicodeCMap()))
	if err != nil {
		return 0, err
	}
	objs.setRaw(toUnicode, cmapStream)

	return fontDict, nil
}

// usedGlyphs returns the drawn glyph indices in ascending order.
func (t *ttfFont) usedGlyphs() []uint16 {
	gids := make([]uint16, 0, len(t.used))
	for gid := range t.used {
		gids = append(gids, gid)
	}
	sort.Slice(gids, func(i, j int) bool { return gids[i] < gids[j] })
	return gids
}

// widthArray builds the /W entry, grouping consecutive glyph indices so the
// array stays compact.
func (t *ttfFont) widthArray() string {
	gids := t.usedGlyphs()
	if len(gids) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < len(gids); {
		j := i
		for j+1 < len(gids) && gids[j+1] == gids[j]+1 {
			j++
		}
		fmt.Fprintf(&sb, "%d [", gids[i])
		for k := i; k <= j; k++ {
			if k > i {
				sb.WriteByte(' ')
			}
			fmt.Fprintf(&sb, "%d", int(t.advance(gids[k])+0.5))
		}
		sb.WriteString("] ")
		i = j + 1
	}
	sb.WriteByte(']')
	return sb.String()
}

// toUnicodeCMap maps glyph indices back to characters. Without it the text
// renders correctly but cannot be searched or copied out of the document.
func (t *ttfFont) toUnicodeCMap() string {
	var sb strings.Builder
	sb.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n")
	sb.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	sb.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	sb.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")

	gids := t.usedGlyphs()
	// The format allows at most 100 mappings per block.
	const perBlock = 100
	for start := 0; start < len(gids); start += perBlock {
		end := start + perBlock
		if end > len(gids) {
			end = len(gids)
		}
		fmt.Fprintf(&sb, "%d beginbfchar\n", end-start)
		for _, gid := range gids[start:end] {
			fmt.Fprintf(&sb, "<%04X> <", gid)
			for _, u := range utf16.Encode([]rune{t.used[gid]}) {
				fmt.Fprintf(&sb, "%04X", u)
			}
			sb.WriteString(">\n")
		}
		sb.WriteString("endbfchar\n")
	}
	sb.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return sb.String()
}
