// Command report writes report.pdf, demonstrating everything the library can
// do: the built-in fonts, wrapping, alignment, colour and an embedded
// TrueType font for text outside Western Europe.
//
// Usage:
//
//	go run ./examples/report [-font path/to/font.ttf] [-o report.pdf]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	pdfwriter "github.com/eli-eric/pdf-writer"
)

func main() {
	out := flag.String("o", "report.pdf", "file to write")
	fontPath := flag.String("font", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"TrueType font used for the Unicode page")
	flag.Parse()

	d := pdfwriter.New()
	d.SetTitle("pdf-writer example report")
	d.SetAuthor("ELI ERIC")

	builtinPage(d)
	unicodePage(d, *fontPath)

	if err := d.Save(*out); err != nil {
		log.Fatal(err)
	}
	info, err := os.Stat(*out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d pages, %.1f kB)\n", *out, d.PageCount(), float64(info.Size())/1000)
}

// builtinPage uses only the fonts every PDF viewer already has, so it needs no
// font file and adds nothing to the file size.
func builtinPage(d *pdfwriter.Doc) {
	d.AddPage()

	d.SetFont(pdfwriter.HelveticaBold, 22)
	d.Text(20, 30, "pdf-writer")

	d.SetTextColor(90, 90, 90)
	d.SetFont(pdfwriter.Helvetica, 11)
	d.Text(20, 38, "Text-only PDFs, standard library only")

	// Right-align the date against the opposite margin.
	d.TextRight(pdfwriter.A4Width-20, 30, "10 September 2026")
	d.SetTextColor(0, 0, 0)

	d.SetFont(pdfwriter.Helvetica, 11)
	y := d.Paragraph(20, 55, 170, 5.5,
		"This paragraph is wrapped automatically to the column width using the "+
			"real font metrics, so the right edge is measured rather than guessed. "+
			"Paragraph returns the next free baseline, which makes it easy to stack "+
			"blocks of text without tracking coordinates by hand.")

	y += 6
	d.SetFont(pdfwriter.HelveticaBold, 12)
	d.Text(20, y, "The twelve built-in fonts")
	y += 8

	for _, f := range []*pdfwriter.Font{
		pdfwriter.Helvetica, pdfwriter.HelveticaBold, pdfwriter.HelveticaOblique,
		pdfwriter.Times, pdfwriter.TimesBold, pdfwriter.TimesItalic,
		pdfwriter.Courier, pdfwriter.CourierBold,
	} {
		d.SetFont(f, 11)
		d.Text(25, y, f.Name()+" — Grüße, café, naïve, 20 €")
		y += 6
	}

	y += 6
	d.SetFont(pdfwriter.HelveticaBold, 12)
	d.Text(20, y, "A simple table, aligned with TextWidth")
	y += 8

	for _, row := range [][2]string{
		{"Laser pulse energy", "12.4 J"},
		{"Repetition rate", "10 Hz"},
		{"Pulse duration", "28 fs"},
		{"Peak power", "443 TW"},
	} {
		d.SetFont(pdfwriter.Helvetica, 11)
		d.Text(25, y, row[0])
		// Numbers read better right-aligned on a common edge.
		d.SetFont(pdfwriter.Courier, 11)
		d.TextRight(120, y, row[1])
		y += 6
	}
}

// unicodePage embeds a TrueType font, which is what makes Czech and other
// scripts possible.
func unicodePage(d *pdfwriter.Doc, fontPath string) {
	d.AddPage()
	d.SetFont(pdfwriter.HelveticaBold, 16)
	d.Text(20, 30, "Embedded TrueType font")

	font, err := d.AddTTFFont("DejaVuSans", fontPath)
	if err != nil {
		// Not fatal: the rest of the document is still worth writing.
		d.SetFont(pdfwriter.Helvetica, 11)
		d.Paragraph(20, 42, 170, 5.5, "This page needs a TrueType font. "+
			fmt.Sprintf("%v", err)+" Pass one with -font.")
		return
	}

	d.SetFont(pdfwriter.Helvetica, 11)
	d.SetTextColor(90, 90, 90)
	d.Text(20, 42, "Built-in fonts stop at Western Europe. Embedding a .ttf lifts that limit:")
	d.SetTextColor(0, 0, 0)

	d.SetFont(font, 13)
	y := d.Paragraph(20, 55, 170, 7,
		"Příliš žluťoučký kůň úpěl ďábelské ódy. "+
			"Řeřicha, ěščřžýáíé — plná česká sada diakritiky.")

	y += 6
	d.SetFont(font, 12)
	for _, line := range []string{
		"Czech:   Příliš žluťoučký kůň úpěl ďábelské ódy.",
		"Polish:  Zażółć gęślą jaźń.",
		"Greek:   Ξεσκεπάζω την ψυχοφθόρα βδελυγμία.",
		"Russian: Съешь же ещё этих мягких французских булок.",
	} {
		d.Text(20, y, line)
		y += 7
	}
}
