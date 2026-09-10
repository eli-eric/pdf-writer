package pdfwriter_test

import (
	"bytes"
	"fmt"
	"log"

	pdfwriter "github.com/eli-eric/pdf-writer"
)

// The shortest useful document: one page, one built-in font, one line of text.
func Example() {
	d := pdfwriter.New()
	d.AddPage()
	d.SetFont(pdfwriter.Helvetica, 12)
	d.Text(20, 20, "Hello, world")

	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		log.Fatal(err)
	}
	// Nothing is embedded when using the built-in fonts, so the whole file
	// fits in well under a kilobyte.
	fmt.Printf("%d page, under 1 kB: %v\n", d.PageCount(), buf.Len() < 1000)
	// Output: 1 page, under 1 kB: true
}

// Paragraph wraps text to a column width and reports the next free baseline,
// so blocks of text can be stacked without tracking coordinates by hand.
func ExampleDoc_Paragraph() {
	d := pdfwriter.New()
	d.AddPage()
	d.SetFont(pdfwriter.Helvetica, 11)

	y := d.Paragraph(20, 30, 80, 5, "Wrapping uses the real font metrics, "+
		"so the column edge is measured rather than guessed.")
	fmt.Printf("next baseline: %.0f mm\n", y)
	// Output: next baseline: 45 mm
}

// TextWidth measures text in the current font and size, which is what makes
// right-aligned and centred text possible.
func ExampleDoc_TextWidth() {
	d := pdfwriter.New()
	d.AddPage()
	d.SetFont(pdfwriter.Helvetica, 12)

	fmt.Printf("%.2f mm\n", d.TextWidth("Hello"))
	// Output: 9.64 mm
}

// A built-in font reports an error rather than silently drawing the wrong
// glyph for a character WinAnsiEncoding cannot represent.
func ExampleDoc_Err() {
	d := pdfwriter.New()
	d.AddPage()
	d.SetFont(pdfwriter.Helvetica, 12)
	d.Text(20, 20, "Příliš žluťoučký kůň")

	fmt.Println(d.Err())
	// Output: pdfwriter: the built-in font Helvetica cannot encode 'ř' (U+0159); embed a TrueType font with AddTTFFont to use this character
}

// Embedding a TrueType font lifts the 256-character limit of the built-in
// fonts, giving full Unicode coverage.
func ExampleDoc_AddTTFFont() {
	d := pdfwriter.New()

	font, err := d.AddTTFFont("DejaVuSans", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	if err != nil {
		log.Fatal(err)
	}

	d.AddPage()
	d.SetFont(font, 12)
	d.Text(20, 20, "Příliš žluťoučký kůň úpěl ďábelské ódy.")

	if err := d.Save("czech.pdf"); err != nil {
		log.Fatal(err)
	}
}

// Text can be aligned against an edge or centred on a point, which is enough
// to lay out a simple table.
func ExampleDoc_TextRight() {
	d := pdfwriter.New()
	d.AddPage()

	rows := [][2]string{{"Pulse energy", "12.4 J"}, {"Repetition rate", "10 Hz"}}
	y := 30.0
	for _, row := range rows {
		d.SetFont(pdfwriter.Helvetica, 11)
		d.Text(25, y, row[0])
		// Numbers read better right-aligned on a common edge.
		d.SetFont(pdfwriter.Courier, 11)
		d.TextRight(120, y, row[1])
		y += 6
	}

	if err := d.Err(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("laid out", len(rows), "rows")
	// Output: laid out 2 rows
}
