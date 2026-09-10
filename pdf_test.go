package pdfwriter

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// czech exercises the diacritics that WinAnsiEncoding cannot represent.
const czech = "Příliš žluťoučký kůň úpěl ďábelské ódy."

// ttfPath is a font with full Czech coverage, used by the embedding tests.
const ttfPath = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"

// render builds a document and returns the PDF bytes, failing the test on error.
func render(t *testing.T, fn func(d *Doc)) []byte {
	t.Helper()
	d := New()
	d.SetCreationDate(time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC))
	fn(d)
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

// checkStructure verifies the file envelope and that every cross-reference
// table entry points at the object it claims to. A wrong offset is the classic
// way a hand-written PDF breaks, and viewers report it only as a vague error.
func checkStructure(t *testing.T, pdf []byte) {
	t.Helper()
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4\n")) {
		t.Error("missing PDF header")
	}
	if !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Error("missing end-of-file trailer")
	}

	// startxref must point at the xref keyword.
	m := regexp.MustCompile(`startxref\n(\d+)\n%%EOF`).FindSubmatch(pdf)
	if m == nil {
		t.Fatal("no startxref")
	}
	start, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	if start >= len(pdf) || !bytes.HasPrefix(pdf[start:], []byte("xref\n")) {
		t.Fatalf("startxref %d does not point at an xref table", start)
	}

	// Each 20-byte entry after the free head must locate "N 0 obj".
	body := pdf[start:]
	lines := bytes.Split(body, []byte("\n"))
	size := regexp.MustCompile(`/Size (\d+)`).FindSubmatch(pdf)
	if size == nil {
		t.Fatal("no /Size in trailer")
	}
	n, _ := strconv.Atoi(string(size[1]))
	for i := 1; i < n; i++ {
		entry := lines[2+i] // skip "xref", "0 N", and the free entry
		off, err := strconv.Atoi(strings.Fields(string(entry))[0])
		if err != nil {
			t.Fatalf("object %d: bad xref entry %q", i, entry)
		}
		want := fmt.Sprintf("%d 0 obj", i)
		if off >= len(pdf) || !bytes.HasPrefix(pdf[off:], []byte(want)) {
			t.Errorf("object %d: xref offset %d does not point at %q", i, off, want)
		}
	}
}

// extract runs the text back out of a PDF with poppler, which parses the file
// independently of the code that wrote it. Tests calling this skip when
// poppler is absent.
func extract(t *testing.T, pdf []byte) string {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed; skipping round-trip check")
	}
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	// -enc UTF-8 is essential rather than cosmetic: older poppler builds,
	// including the one on the Windows CI runner, default to Latin-1 output.
	// That silently mangles the accents these tests are checking and drops
	// characters Latin-1 has no room for, such as the euro sign.
	out, err := exec.Command("pdftotext", "-q", "-enc", "UTF-8", path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext: %v", err)
	}
	return string(out)
}

func TestStructure(t *testing.T) {
	pdf := render(t, func(d *Doc) {
		d.SetTitle("Structure test")
		d.SetAuthor("pdf-writer")
		d.AddPage()
		d.SetFont(Helvetica, 12)
		d.Text(20, 20, "Hello, world")
	})
	checkStructure(t, pdf)
}

func TestStructureUncompressed(t *testing.T) {
	// The uncompressed path shifts every object offset, so it needs its own
	// cross-reference check.
	d := New()
	d.SetCompress(false)
	d.AddPage()
	d.SetFont(Times, 11)
	d.Text(20, 20, "plain")
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	checkStructure(t, buf.Bytes())
	if !bytes.Contains(buf.Bytes(), []byte("(plain) Tj")) {
		t.Error("uncompressed stream should contain readable text operators")
	}
}

func TestRoundTripBuiltinFont(t *testing.T) {
	const line = "The quick brown fox jumps over the lazy dog."
	pdf := render(t, func(d *Doc) {
		d.AddPage()
		d.SetFont(Helvetica, 12)
		d.Text(20, 20, line)
	})
	if got := extract(t, pdf); !strings.Contains(got, line) {
		t.Errorf("extracted text %q does not contain %q", got, line)
	}
}

// TestRoundTripWinAnsiAccents covers the Western European characters the
// built-in fonts can encode, including the ones WinAnsi puts in 0x80-0x9F.
func TestRoundTripWinAnsiAccents(t *testing.T) {
	const line = "Grüße café naïve — 20 € «quoted» ¿cómo?"
	pdf := render(t, func(d *Doc) {
		d.AddPage()
		d.SetFont(Helvetica, 12)
		d.Text(20, 20, line)
	})
	got := extract(t, pdf)
	for _, r := range []string{"Grüße", "café", "naïve", "€", "«quoted»", "¿cómo?"} {
		if !strings.Contains(got, r) {
			t.Errorf("extracted text %q is missing %q", got, r)
		}
	}
}

// TestBuiltinFontRejectsCzech pins the documented limitation: rather than
// emitting silently wrong glyphs, the library reports the problem and names
// the fix.
func TestBuiltinFontRejectsCzech(t *testing.T) {
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)
	d.Text(20, 20, czech)

	err := d.Err()
	if err == nil {
		t.Fatal("expected an error for characters outside WinAnsiEncoding")
	}
	if !strings.Contains(err.Error(), "AddTTFFont") {
		t.Errorf("error should point at the fix, got: %v", err)
	}
	// The error must also surface at the end of the pipeline.
	if _, err := d.WriteTo(&bytes.Buffer{}); err == nil {
		t.Error("WriteTo should return the pending error")
	}
}

func TestRoundTripEmbeddedTTF(t *testing.T) {
	if _, err := os.Stat(ttfPath); err != nil {
		t.Skipf("%s not available", ttfPath)
	}
	d := New()
	d.SetCreationDate(time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC))
	font, err := d.AddTTFFont("DejaVuSans", ttfPath)
	if err != nil {
		t.Fatalf("AddTTFFont: %v", err)
	}
	d.AddPage()
	d.SetFont(font, 12)
	d.Text(20, 20, czech)

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	pdf := buf.Bytes()
	checkStructure(t, pdf)

	// Recovering the text proves the glyph indices, the /W widths and the
	// ToUnicode reverse map all agree.
	if got := extract(t, pdf); !strings.Contains(got, czech) {
		t.Errorf("extracted text %q does not contain %q", got, czech)
	}
}

func TestTTFRejectsUnsupportedFormats(t *testing.T) {
	d := New()
	// 'OTTO' marks PostScript outlines, which cannot go in a FontFile2.
	if _, err := d.AddTTFFontData("otf", []byte("OTTO\x00\x00\x00\x00")); err == nil {
		t.Error("expected OpenType/CFF fonts to be rejected")
	} else if !strings.Contains(err.Error(), "PostScript") {
		t.Errorf("error should explain the format, got: %v", err)
	}
	if _, err := d.AddTTFFontData("junk", []byte("not a font")); err == nil {
		t.Error("expected garbage input to be rejected")
	}
	if _, err := d.AddTTFFontData("short", []byte{0x00}); err == nil {
		t.Error("expected truncated input to be rejected")
	}
}

func TestTTFDuplicateName(t *testing.T) {
	if _, err := os.Stat(ttfPath); err != nil {
		t.Skipf("%s not available", ttfPath)
	}
	d := New()
	if _, err := d.AddTTFFont("Body", ttfPath); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddTTFFont("Body", ttfPath); err == nil {
		t.Error("expected a duplicate font name to be rejected")
	}
}

// TestTextWidth checks measurement against Adobe's published Helvetica
// metrics: "Hello" is H+e+l+l+o = 722+556+222+222+556 = 2278 units of a
// 1000-unit em.
func TestTextWidth(t *testing.T) {
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)

	wantPt := 2278.0 * 12 / 1000
	if got := d.textWidthPt("Hello"); absDiff(got, wantPt) > 1e-9 {
		t.Errorf("textWidthPt = %v, want %v", got, wantPt)
	}
	// The same width expressed in millimetres.
	wantMM := wantPt * 25.4 / 72
	if got := d.TextWidth("Hello"); absDiff(got, wantMM) > 1e-9 {
		t.Errorf("TextWidth = %v mm, want %v mm", got, wantMM)
	}
	// Width must scale linearly with font size.
	d.SetFont(Helvetica, 24)
	if got := d.textWidthPt("Hello"); absDiff(got, wantPt*2) > 1e-9 {
		t.Errorf("doubling the size gave %v, want %v", got, wantPt*2)
	}
	// Courier is monospaced at 600 units per character.
	d.SetFont(Courier, 10)
	if got := d.textWidthPt("abcd"); absDiff(got, 4*600*10/1000.0) > 1e-9 {
		t.Errorf("Courier width = %v, want 24", got)
	}
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func TestAlignment(t *testing.T) {
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)
	const s = "right"
	w := d.TextWidth(s)

	d.TextRight(100, 20, s)
	d.TextCenter(100, 30, s)
	if d.Err() != nil {
		t.Fatal(d.Err())
	}

	content := d.pages[0].buf.String()
	// TextRight ends at 100 mm, so it starts at 100-w.
	wantRight := ftoa(mmToPt(100 - w))
	if !strings.Contains(content, wantRight+" ") {
		t.Errorf("TextRight should start at %s pt; content:\n%s", wantRight, content)
	}
	wantCenter := ftoa(mmToPt(100 - w/2))
	if !strings.Contains(content, wantCenter+" ") {
		t.Errorf("TextCenter should start at %s pt; content:\n%s", wantCenter, content)
	}
}

func TestWrap(t *testing.T) {
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)

	lines := d.wrap("alpha beta gamma delta epsilon zeta eta theta", 40)
	if len(lines) < 2 {
		t.Fatalf("expected the text to wrap, got %q", lines)
	}
	limit := mmToPt(40)
	for _, line := range lines {
		if w := d.textWidthPt(line); w > limit {
			t.Errorf("line %q is %v pt wide, over the %v pt limit", line, w, limit)
		}
	}
	// No word may be lost or duplicated by wrapping.
	if got, want := strings.Join(strings.Fields(strings.Join(lines, " ")), " "),
		"alpha beta gamma delta epsilon zeta eta theta"; got != want {
		t.Errorf("wrapping changed the text:\n got %q\nwant %q", got, want)
	}
}

func TestWrapKeepsBlankLines(t *testing.T) {
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)

	lines := d.wrap("first\n\nthird", 100)
	want := []string{"first", "", "third"}
	if len(lines) != len(want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

// TestWrapBreaksLongWord covers a run with no spaces, such as a URL, which
// cannot be wrapped on a space boundary.
func TestWrapBreaksLongWord(t *testing.T) {
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)

	long := strings.Repeat("x", 200)
	lines := d.wrap(long, 40)
	if len(lines) < 2 {
		t.Fatalf("expected the run to be broken up, got %d line(s)", len(lines))
	}
	limit := mmToPt(40)
	for _, line := range lines {
		if w := d.textWidthPt(line); w > limit {
			t.Errorf("line is %v pt wide, over the %v pt limit", w, limit)
		}
	}
	if got := strings.Join(lines, ""); got != long {
		t.Error("breaking the run lost or added characters")
	}
}

func TestParagraphReturnsNextBaseline(t *testing.T) {
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)

	const lineHeight = 6.0
	y := d.Paragraph(20, 30, 170, lineHeight, "one line only")
	if want := 30 + lineHeight; absDiff(y, want) > 1e-9 {
		t.Errorf("y = %v after one line, want %v", y, want)
	}
	if d.Err() != nil {
		t.Fatal(d.Err())
	}
}

func TestMultiplePages(t *testing.T) {
	pdf := render(t, func(d *Doc) {
		d.SetFont(Helvetica, 12)
		for i := 1; i <= 3; i++ {
			d.AddPage()
			d.Text(20, 20, fmt.Sprintf("page %d", i))
		}
	})
	checkStructure(t, pdf)
	if !bytes.Contains(pdf, []byte("/Count 3")) {
		t.Error("page tree should report three pages")
	}
	got := extract(t, pdf)
	for i := 1; i <= 3; i++ {
		if !strings.Contains(got, fmt.Sprintf("page %d", i)) {
			t.Errorf("extracted text is missing page %d:\n%s", i, got)
		}
	}
}

// TestFontsAreSharedAcrossPages checks that repeated SetFont calls produce one
// font resource rather than one per use.
func TestFontsAreSharedAcrossPages(t *testing.T) {
	d := New()
	for i := 0; i < 3; i++ {
		d.AddPage()
		d.SetFont(Helvetica, 12)
		d.Text(20, 20, "x")
		d.SetFont(TimesBold, 14)
		d.Text(20, 30, "y")
	}
	if len(d.fonts) != 2 {
		t.Errorf("registered %d fonts, want 2", len(d.fonts))
	}
}

func TestMediaBoxIsA4(t *testing.T) {
	pdf := render(t, func(d *Doc) {
		d.AddPage()
		d.SetFont(Helvetica, 12)
		d.Text(20, 20, "x")
	})
	// 210 x 297 mm in points, to two decimal places.
	if !bytes.Contains(pdf, []byte("/MediaBox [0 0 595.276 841.89]")) {
		t.Error("MediaBox is not A4")
	}
}

func TestErrors(t *testing.T) {
	t.Run("no page", func(t *testing.T) {
		d := New()
		d.SetFont(Helvetica, 12)
		d.Text(20, 20, "x")
		if d.Err() == nil {
			t.Error("drawing without a page should fail")
		}
	})
	t.Run("no font", func(t *testing.T) {
		d := New()
		d.AddPage()
		d.Text(20, 20, "x")
		if d.Err() == nil {
			t.Error("drawing without a font should fail")
		}
	})
	t.Run("no pages at all", func(t *testing.T) {
		d := New()
		if _, err := d.WriteTo(&bytes.Buffer{}); err == nil {
			t.Error("an empty document should not be written")
		}
	})
	t.Run("bad size", func(t *testing.T) {
		d := New()
		d.AddPage()
		d.SetFont(Helvetica, 0)
		if d.Err() == nil {
			t.Error("a non-positive font size should fail")
		}
	})
	t.Run("first error wins", func(t *testing.T) {
		d := New()
		d.AddPage()
		d.SetFont(Helvetica, 12)
		d.Text(20, 20, "č") // unencodable
		first := d.Err()
		d.SetFont(nil, 12) // another failure
		if d.Err() != first {
			t.Error("a later error should not replace the first")
		}
	})
}

func TestEscaping(t *testing.T) {
	// Parentheses and backslashes end or escape a PDF literal string, so
	// they must be quoted in the content stream.
	d := New()
	d.SetCompress(false)
	d.AddPage()
	d.SetFont(Helvetica, 12)
	d.Text(20, 20, `a(b)c\d`)
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`(a\(b\)c\\d) Tj`)) {
		t.Errorf("string was not escaped correctly:\n%s", buf.String())
	}
	checkStructure(t, buf.Bytes())
}

func TestMetadata(t *testing.T) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo not installed")
	}
	pdf := render(t, func(d *Doc) {
		d.SetTitle("Zkouška")
		d.SetAuthor("Jiří")
		d.AddPage()
		d.SetFont(Helvetica, 12)
		d.Text(20, 20, "x")
	})
	path := filepath.Join(t.TempDir(), "meta.pdf")
	if err := os.WriteFile(path, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("pdfinfo", path).Output()
	if err != nil {
		t.Fatalf("pdfinfo: %v", err)
	}
	// Non-ASCII metadata goes out as UTF-16BE, so recovering it intact
	// exercises that path.
	for _, want := range []string{"Zkouška", "Jiří"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("pdfinfo output is missing %q:\n%s", want, out)
		}
	}
}

func TestReproducible(t *testing.T) {
	build := func() []byte {
		return render(t, func(d *Doc) {
			d.AddPage()
			d.SetFont(Helvetica, 12)
			d.Text(20, 20, "same input, same bytes")
		})
	}
	if !bytes.Equal(build(), build()) {
		t.Error("a fixed creation date should give byte-identical output")
	}
}

// format4TTFPath has only a format 4 character map, unlike DejaVu which also
// carries a format 12 one. Format 4 is the most common format in the wild, so
// it needs its own coverage.
const format4TTFPath = "/usr/share/fonts/truetype/crosextra/Carlito-Regular.ttf"

func TestRoundTripFormat4Cmap(t *testing.T) {
	if _, err := os.Stat(format4TTFPath); err != nil {
		t.Skipf("%s not available", format4TTFPath)
	}
	d := New()
	font, err := d.AddTTFFont("Carlito", format4TTFPath)
	if err != nil {
		t.Fatalf("AddTTFFont: %v", err)
	}
	if got := len(font.ttf.cmap); got < 100 {
		t.Errorf("character map has only %d entries; format 4 parsing looks wrong", got)
	}
	d.AddPage()
	d.SetFont(font, 12)
	d.Text(20, 20, czech)

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	checkStructure(t, buf.Bytes())
	if got := extract(t, buf.Bytes()); !strings.Contains(got, czech) {
		t.Errorf("extracted text %q does not contain %q", got, czech)
	}
}

// TestParseCmap4 checks the segment arithmetic directly, with a hand-built
// table covering both ways format 4 can encode a segment: an arithmetic delta,
// and an indirection through glyphIdArray. Getting the glyphIdArray address
// wrong is the classic mistake, because the offset is measured from the
// address of the idRangeOffset entry itself rather than from the table start.
func TestParseCmap4(t *testing.T) {
	const (
		segCount = 3
		subtable = 12 // where the format 4 subtable begins
	)
	b := make([]byte, subtable+44)
	put := func(off int, vals ...uint16) {
		for i, v := range vals {
			binary.BigEndian.PutUint16(b[off+i*2:], v)
		}
	}

	// cmap header: one subtable, Windows/BMP, at offset 12.
	put(0, 0, 1, 3, 1)
	binary.BigEndian.PutUint32(b[8:], subtable)

	put(subtable+0, 4, 44, 0, segCount*2, 0, 0, 0)
	// Segment ends, then starts.
	put(subtable+14, 'C', 'b', 0xFFFF)
	put(subtable+22, 'A', 'a', 0xFFFF)
	// Deltas: segment 0 adds 10; segment 1 uses glyphIdArray so its delta
	// is 0; the terminator is conventional.
	put(subtable+28, 10, 0, 1)
	// idRangeOffset: segment 1's entry sits at subtable+36, and its glyphs
	// start at subtable+40, so the offset is 4.
	put(subtable+34, 0, 4, 0)
	put(subtable+40, 100, 101)

	f := &ttfFont{numGlyphs: 200}
	if err := f.parseCmap(b); err != nil {
		t.Fatalf("parseCmap: %v", err)
	}
	for _, tc := range []struct {
		r    rune
		want uint16
	}{
		{'A', 'A' + 10}, {'B', 'B' + 10}, {'C', 'C' + 10}, // delta path
		{'a', 100}, {'b', 101}, // glyphIdArray path
	} {
		if got, ok := f.cmap[tc.r]; !ok || got != tc.want {
			t.Errorf("cmap[%q] = %d (present %v), want %d", tc.r, got, ok, tc.want)
		}
	}
	// The 0xFFFF terminator is not a real character.
	if _, ok := f.cmap[0xFFFF]; ok {
		t.Error("0xFFFF should not be mapped")
	}
	if len(f.cmap) != 5 {
		t.Errorf("mapped %d characters, want 5", len(f.cmap))
	}
}

func TestSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved.pdf")
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)
	d.Text(20, 20, "saved to disk")
	if err := d.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	checkStructure(t, data)
	if got := extract(t, data); !strings.Contains(got, "saved to disk") {
		t.Errorf("extracted %q", got)
	}
}

func TestSaveReportsPendingError(t *testing.T) {
	d := New()
	d.AddPage()
	d.SetFont(Helvetica, 12)
	d.Text(20, 20, "č") // cannot be encoded
	path := filepath.Join(t.TempDir(), "never.pdf")
	if err := d.Save(path); err == nil {
		t.Error("Save should report the pending error")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("Save should not create a file when it fails")
	}
}

func TestSetTextColor(t *testing.T) {
	d := New()
	d.SetCompress(false)
	d.AddPage()
	d.SetFont(Helvetica, 12)
	d.SetTextColor(255, 0, 0)
	d.Text(20, 20, "red")
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	// Pure red is 1 0 0 in the PDF's 0..1 range.
	if !bytes.Contains(buf.Bytes(), []byte("1 0 0 rg")) {
		t.Errorf("colour operator missing:\n%s", buf.String())
	}
}

func TestFontName(t *testing.T) {
	if got := Helvetica.Name(); got != "Helvetica" {
		t.Errorf("Name() = %q, want %q", got, "Helvetica")
	}
	if got := TimesBoldItalic.Name(); got != "Times-BoldItalic" {
		t.Errorf("Name() = %q, want %q", got, "Times-BoldItalic")
	}
}

// TestTTFNameSanitising checks that a name needing escaping as a PDF name
// object is reduced to safe characters.
func TestTTFNameSanitising(t *testing.T) {
	if _, err := os.Stat(ttfPath); err != nil {
		t.Skipf("%s not available", ttfPath)
	}
	d := New()
	font, err := d.AddTTFFont("My Font (v2)!", ttfPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := font.Name(); got != "MyFontv2" {
		t.Errorf("Name() = %q, want %q", got, "MyFontv2")
	}
}
