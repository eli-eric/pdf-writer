package pdfwriter

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// A4 page dimensions in millimetres.
const (
	A4Width  = 210.0
	A4Height = 297.0
)

// mmToPt converts millimetres to PostScript points, the unit PDF uses
// internally. There are 72 points to an inch and 25.4 millimetres to an inch.
func mmToPt(mm float64) float64 { return mm * 72.0 / 25.4 }

// The 12 text fonts that every conforming PDF viewer provides without
// embedding. They can only render the WinAnsiEncoding character set, which
// covers ASCII and Western Europe. Use [Doc.AddTTFFont] for anything else.
var (
	Helvetica            = stdFont("Helvetica")
	HelveticaBold        = stdFont("Helvetica-Bold")
	HelveticaOblique     = stdFont("Helvetica-Oblique")
	HelveticaBoldOblique = stdFont("Helvetica-BoldOblique")
	Times                = stdFont("Times-Roman")
	TimesBold            = stdFont("Times-Bold")
	TimesItalic          = stdFont("Times-Italic")
	TimesBoldItalic      = stdFont("Times-BoldItalic")
	Courier              = stdFont("Courier")
	CourierBold          = stdFont("Courier-Bold")
	CourierOblique       = stdFont("Courier-Oblique")
	CourierBoldOblique   = stdFont("Courier-BoldOblique")
)

// Font is a handle to a typeface. Obtain one from the package-level variables
// above or from [Doc.AddTTFFont].
type Font struct {
	name   string       // PDF BaseFont name
	widths *[256]uint16 // set for the built-in fonts
	ttf    *ttfFont     // set for embedded TrueType fonts
}

// Name returns the font's PDF name, which is useful in error messages.
func (f *Font) Name() string { return f.name }

func stdFont(name string) *Font {
	w, ok := stdWidths[name]
	if !ok {
		panic("pdfwriter: no metrics for " + name)
	}
	return &Font{name: name, widths: w}
}

// page holds one page's content stream as it is being built.
type page struct {
	buf bytes.Buffer
}

// Doc is a document under construction. Create one with [New].
type Doc struct {
	pages    []*page
	fonts    []*Font        // in resource order; /F1 is fonts[0]
	fontIdx  map[string]int // font name to index in fonts
	font     *Font          // current font
	size     float64        // current size in points
	color    [3]float64     // current fill colour, 0..1 per channel
	compress bool
	title    string
	author   string
	created  time.Time
	err      error // first error seen; returned by Save and WriteTo
}

// New returns an empty A4 document with content-stream compression enabled.
// Call [Doc.AddPage] before drawing.
func New() *Doc {
	return &Doc{
		fontIdx:  make(map[string]int),
		size:     12,
		compress: true,
		created:  time.Now(),
	}
}

// SetTitle sets the document title stored in the PDF metadata.
func (d *Doc) SetTitle(s string) { d.title = s }

// SetAuthor sets the document author stored in the PDF metadata.
func (d *Doc) SetAuthor(s string) { d.author = s }

// SetCreationDate overrides the creation timestamp, which defaults to the
// moment [New] was called. Set it to a fixed value for reproducible output.
func (d *Doc) SetCreationDate(t time.Time) { d.created = t }

// SetCompress enables or disables Flate compression of page content. It is on
// by default; turning it off makes the generated PDF readable in a text editor,
// which helps when debugging.
func (d *Doc) SetCompress(on bool) { d.compress = on }

// Err reports the first error recorded by a drawing call, if any.
func (d *Doc) Err() error { return d.err }

// fail records err if no earlier error is pending.
func (d *Doc) fail(err error) {
	if d.err == nil {
		d.err = err
	}
}

// AddPage appends a new blank A4 page and makes it current.
func (d *Doc) AddPage() {
	d.pages = append(d.pages, &page{})
}

// PageCount returns the number of pages added so far.
func (d *Doc) PageCount() int { return len(d.pages) }

// cur returns the current page, recording an error if there is none.
func (d *Doc) cur() *page {
	if len(d.pages) == 0 {
		d.fail(errors.New("pdfwriter: no page; call AddPage first"))
		return nil
	}
	return d.pages[len(d.pages)-1]
}

// SetFont selects the font and size, in points, for subsequent text.
func (d *Doc) SetFont(f *Font, sizePt float64) {
	if f == nil {
		d.fail(errors.New("pdfwriter: nil font"))
		return
	}
	if sizePt <= 0 {
		d.fail(fmt.Errorf("pdfwriter: font size must be positive, got %v", sizePt))
		return
	}
	d.font = f
	d.size = sizePt
	d.register(f)
}

// register assigns f a resource slot, reusing one if the font is already known.
func (d *Doc) register(f *Font) int {
	if i, ok := d.fontIdx[f.name]; ok {
		return i
	}
	i := len(d.fonts)
	d.fonts = append(d.fonts, f)
	d.fontIdx[f.name] = i
	return i
}

// SetTextColor sets the text colour from 8-bit RGB components.
func (d *Doc) SetTextColor(r, g, b uint8) {
	d.color = [3]float64{float64(r) / 255, float64(g) / 255, float64(b) / 255}
}

// ftoa formats a number for the PDF content stream, avoiding exponents and
// pointless trailing zeros.
func ftoa(v float64) string {
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

// Save writes the document to a file, replacing any existing one.
func (d *Doc) Save(path string) error {
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// WriteTo serialises the document. It returns the first error recorded by any
// earlier call, so checking here is enough.
func (d *Doc) WriteTo(w io.Writer) (int64, error) {
	if d.err != nil {
		return 0, d.err
	}
	if len(d.pages) == 0 {
		return 0, errors.New("pdfwriter: document has no pages")
	}
	buf, err := d.build()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(buf)
	return int64(n), err
}

// objects collects indirect objects, whose numbers start at 1.
type objects struct {
	body [][]byte
}

// reserve allocates an object number whose contents are supplied later. This
// lets objects refer to each other regardless of the order they are built in.
func (o *objects) reserve() int {
	o.body = append(o.body, nil)
	return len(o.body)
}

// set fills in a previously reserved object.
func (o *objects) set(num int, content string) {
	o.body[num-1] = []byte(content)
}

// setRaw fills in a reserved object with binary content.
func (o *objects) setRaw(num int, content []byte) {
	o.body[num-1] = content
}

// add reserves and fills an object in one step.
func (o *objects) add(content string) int {
	n := o.reserve()
	o.set(n, content)
	return n
}

// build assembles the complete PDF file.
func (d *Doc) build() ([]byte, error) {
	var objs objects
	catalog := objs.reserve()
	pagesNode := objs.reserve()

	// Font objects come before the pages so the shared resource dictionary
	// on the page tree node can reference them.
	fontRefs := make([]int, len(d.fonts))
	for i, f := range d.fonts {
		var err error
		if fontRefs[i], err = d.writeFont(&objs, f); err != nil {
			return nil, err
		}
	}

	var resources strings.Builder
	resources.WriteString("<< /Font << ")
	for i, ref := range fontRefs {
		fmt.Fprintf(&resources, "/F%d %d 0 R ", i+1, ref)
	}
	resources.WriteString(">> >>")

	mediaBox := fmt.Sprintf("[0 0 %s %s]", ftoa(mmToPt(A4Width)), ftoa(mmToPt(A4Height)))

	kids := make([]string, 0, len(d.pages))
	for _, p := range d.pages {
		stream, err := d.stream(p.buf.Bytes())
		if err != nil {
			return nil, err
		}
		contents := objs.reserve()
		objs.setRaw(contents, stream)
		pageNum := objs.add(fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /Contents %d 0 R >>", pagesNode, contents))
		kids = append(kids, fmt.Sprintf("%d 0 R", pageNum))
	}

	objs.set(pagesNode, fmt.Sprintf(
		"<< /Type /Pages /Kids [%s] /Count %d /MediaBox %s /Resources %s >>",
		strings.Join(kids, " "), len(d.pages), mediaBox, resources.String()))
	objs.set(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesNode))

	info := objs.add(fmt.Sprintf("<< /Producer %s /Title %s /Author %s /CreationDate (%s) >>",
		textString("pdf-writer"), textString(d.title), textString(d.author),
		pdfDate(d.created)))

	return serialise(&objs, catalog, info), nil
}

// stream wraps content in a stream object, compressing it when enabled.
func (d *Doc) stream(content []byte) ([]byte, error) {
	filter := ""
	if d.compress {
		var zbuf bytes.Buffer
		zw := zlib.NewWriter(&zbuf)
		if _, err := zw.Write(content); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		content = zbuf.Bytes()
		filter = " /Filter /FlateDecode"
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "<< /Length %d%s >>\nstream\n", len(content), filter)
	out.Write(content)
	out.WriteString("\nendstream")
	return out.Bytes(), nil
}

// serialise lays the objects out in a file with a cross-reference table.
func serialise(objs *objects, catalog, info int) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	// A comment with high bytes marks the file as binary for tools that
	// would otherwise mangle line endings in transit.
	out.Write([]byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'})

	offsets := make([]int, len(objs.body))
	for i, body := range objs.body {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", i+1)
		out.Write(body)
		out.WriteString("\nendobj\n")
	}

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(objs.body)+1)
	// Every entry is exactly 20 bytes wide, as the format requires.
	out.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\n",
		len(objs.body)+1, catalog, info)
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", xref)
	return out.Bytes()
}

// escapeLiteral renders bytes as a PDF literal string, escaping the characters
// that would otherwise end the string or be altered in transit.
func escapeLiteral(b []byte) string {
	var sb strings.Builder
	sb.WriteByte('(')
	for _, c := range b {
		switch {
		case c == '\\' || c == '(' || c == ')':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		case c < 32 || c > 126:
			fmt.Fprintf(&sb, "\\%03o", c)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteByte(')')
	return sb.String()
}

// textString encodes metadata. PDF text strings are PDFDocEncoded unless they
// begin with a byte-order mark, so anything beyond ASCII goes out as UTF-16BE.
func textString(s string) string {
	ascii := true
	for _, r := range s {
		if r > 126 || r < 32 {
			ascii = false
			break
		}
	}
	if ascii {
		return escapeLiteral([]byte(s))
	}
	var sb strings.Builder
	sb.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&sb, "%04X", u)
	}
	sb.WriteByte('>')
	return sb.String()
}

// pdfDate formats a timestamp as PDF's D:YYYYMMDDHHmmSSOHH'mm' form.
func pdfDate(t time.Time) string {
	_, offset := t.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("D:%s%s%02d'%02d'",
		t.Format("20060102150405"), sign, offset/3600, (offset%3600)/60)
}
