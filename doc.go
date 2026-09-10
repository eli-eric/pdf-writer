// Package pdfwriter writes simple text-only PDF documents.
//
// It has no dependencies outside the Go standard library. The scope is
// deliberately narrow: A4 pages and text. There are no shapes, images or
// tables.
//
// # Getting started
//
// A minimal document:
//
//	d := pdfwriter.New()
//	d.AddPage()
//	d.SetFont(pdfwriter.Helvetica, 12)
//	d.Text(20, 20, "Hello, world")
//
//	if err := d.Save("hello.pdf"); err != nil {
//		log.Fatal(err)
//	}
//
// # Coordinates and units
//
// All coordinates are millimetres measured from the top-left corner of the
// page, so x grows rightwards and y grows downwards, the way you would measure
// a sheet of paper. For text, y is the baseline. Font sizes are in points, as
// is conventional in typography. The page is [A4Width] by [A4Height]
// millimetres.
//
// # Fonts
//
// The twelve fonts built into every PDF viewer, [Helvetica] and its relatives,
// need no font file and add nothing to the output size. They can only encode
// WinAnsiEncoding, however, which covers ASCII and Western Europe. Text
// outside that range, such as Czech, Polish, Greek or Cyrillic, needs a
// TrueType font embedded with [Doc.AddTTFFont] or [Doc.AddTTFFontData].
//
// Rather than emit the wrong glyph silently, a built-in font asked for a
// character it cannot encode records an error naming both the character and
// the fix.
//
// # Error handling
//
// Drawing calls do not return errors. Instead the first failure is remembered
// and returned by [Doc.Save] or [Doc.WriteTo], so a page of layout code needs
// one error check at the end rather than one per line. [Doc.Err] reports the
// pending error if you want to look sooner.
package pdfwriter
