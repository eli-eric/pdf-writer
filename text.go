package pdfwriter

import (
	"errors"
	"fmt"
	"strings"
)

// Text draws s with its baseline at (x, y), measured in millimetres from the
// top-left corner of the page. Newlines in s are not interpreted; use
// [Doc.Paragraph] for text that should wrap.
func (d *Doc) Text(x, y float64, s string) {
	p := d.cur()
	if p == nil {
		return
	}
	if d.font == nil {
		d.fail(errors.New("pdfwriter: no font; call SetFont first"))
		return
	}
	operand, err := d.encode(s)
	if err != nil {
		d.fail(err)
		return
	}
	idx := d.fontIdx[d.font.name]
	fmt.Fprintf(&p.buf, "%s %s %s rg\nBT\n/F%d %s Tf\n1 0 0 1 %s %s Tm\n%s Tj\nET\n",
		ftoa(d.color[0]), ftoa(d.color[1]), ftoa(d.color[2]),
		idx+1, ftoa(d.size),
		ftoa(mmToPt(x)), ftoa(mmToPt(A4Height-y)),
		operand)
}

// TextRight draws s so that it ends at x, right-aligning it against that edge.
func (d *Doc) TextRight(x, y float64, s string) {
	d.Text(x-d.TextWidth(s), y, s)
}

// TextCenter draws s centred horizontally on x.
func (d *Doc) TextCenter(x, y float64, s string) {
	d.Text(x-d.TextWidth(s)/2, y, s)
}

// encode turns s into a PDF string operand for the current font: a literal
// string of WinAnsi bytes for the built-in fonts, or a hexadecimal string of
// glyph indices for an embedded TrueType font.
func (d *Doc) encode(s string) (string, error) {
	f := d.font
	if f.ttf != nil {
		var sb strings.Builder
		sb.WriteByte('<')
		for _, r := range s {
			gid, ok := f.ttf.glyph(r)
			if !ok {
				return "", fmt.Errorf(
					"pdfwriter: font %s has no glyph for %q (U+%04X)",
					f.name, r, r)
			}
			f.ttf.used[gid] = r
			fmt.Fprintf(&sb, "%04X", gid)
		}
		sb.WriteByte('>')
		return sb.String(), nil
	}

	buf := make([]byte, 0, len(s))
	for _, r := range s {
		b, ok := winAnsiEncode(r)
		if !ok {
			return "", fmt.Errorf(
				"pdfwriter: the built-in font %s cannot encode %q (U+%04X); "+
					"embed a TrueType font with AddTTFFont to use this character",
				f.name, r, r)
		}
		buf = append(buf, b)
	}
	return escapeLiteral(buf), nil
}

// TextWidth returns how wide s would be in millimetres in the current font and
// size. Characters the font cannot represent are skipped, so measure text you
// intend to draw and check [Doc.Err] afterwards.
func (d *Doc) TextWidth(s string) float64 {
	if d.font == nil {
		return 0
	}
	return mmPerPt(d.textWidthPt(s))
}

// mmPerPt converts points back to millimetres.
func mmPerPt(pt float64) float64 { return pt * 25.4 / 72.0 }

// textWidthPt sums the advance widths of s in points. Glyph space is one
// thousandth of the font size, so the total scales by size/1000.
func (d *Doc) textWidthPt(s string) float64 {
	f := d.font
	var units float64
	for _, r := range s {
		switch {
		case f.ttf != nil:
			if gid, ok := f.ttf.glyph(r); ok {
				units += f.ttf.advance(gid)
			}
		default:
			if b, ok := winAnsiEncode(r); ok {
				units += float64(f.widths[b])
			}
		}
	}
	return units * d.size / 1000.0
}

// Paragraph draws text in a column of the given width, wrapping on spaces and
// breaking on newlines. Successive baselines sit lineHeight millimetres apart.
// It returns the baseline the next line would occupy, which makes it easy to
// stack blocks of text:
//
//	y = d.Paragraph(20, y, 170, 6, intro)
//	y = d.Paragraph(20, y+4, 170, 6, body)
func (d *Doc) Paragraph(x, y, width, lineHeight float64, text string) float64 {
	if d.font == nil {
		d.fail(errors.New("pdfwriter: no font; call SetFont first"))
		return y
	}
	for _, line := range d.wrap(text, width) {
		if line != "" {
			d.Text(x, y, line)
		}
		y += lineHeight
	}
	return y
}

// wrap breaks text into lines that each fit within width millimetres.
func (d *Doc) wrap(text string, width float64) []string {
	limit := mmToPt(width)
	var out []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "") // preserve blank lines
			continue
		}
		line := ""
		for _, word := range words {
			candidate := word
			if line != "" {
				candidate = line + " " + word
			}
			if d.textWidthPt(candidate) <= limit || line == "" {
				// A word that overflows on its own still has to go
				// somewhere, so place it and let breakWord split it.
				line = candidate
				continue
			}
			out = append(out, line)
			line = word
		}
		if line != "" {
			out = append(out, line)
		}
	}

	// Split any line still too wide, which happens with unbroken runs such
	// as long URLs or identifiers.
	var final []string
	for _, line := range out {
		if d.textWidthPt(line) <= limit {
			final = append(final, line)
			continue
		}
		final = append(final, d.breakWord(line, limit)...)
	}
	return final
}

// breakWord chops an over-long line at rune boundaries.
func (d *Doc) breakWord(s string, limitPt float64) []string {
	var out []string
	line := ""
	for _, r := range s {
		candidate := line + string(r)
		if d.textWidthPt(candidate) > limitPt && line != "" {
			out = append(out, line)
			line = string(r)
			continue
		}
		line = candidate
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// writeFont emits the objects describing f and returns the object number of
// its font dictionary.
func (d *Doc) writeFont(objs *objects, f *Font) (int, error) {
	if f.ttf != nil {
		return d.writeTTFFont(objs, f)
	}
	return objs.add(fmt.Sprintf(
		"<< /Type /Font /Subtype /Type1 /BaseFont /%s /Encoding /WinAnsiEncoding >>",
		f.name)), nil
}
