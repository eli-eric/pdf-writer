# pdf-writer

[![Go Reference](https://pkg.go.dev/badge/github.com/eli-eric/pdf-writer.svg)](https://pkg.go.dev/github.com/eli-eric/pdf-writer)
[![CI](https://github.com/eli-eric/pdf-writer/actions/workflows/ci.yml/badge.svg)](https://github.com/eli-eric/pdf-writer/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/eli-eric/pdf-writer)](https://goreportcard.com/report/github.com/eli-eric/pdf-writer)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A small Go package for writing text-only PDFs. **No dependencies outside the
standard library.**

The scope is deliberately narrow, because that is what keeps it small enough to
read in one sitting:

- A4 pages, portrait
- text, in the twelve built-in PDF fonts or an embedded TrueType font
- automatic word wrapping, left/right/centre alignment, text colour

There are no shapes, images, tables or page templates. If you need those, use
[gopdf](https://github.com/signintech/gopdf) or
[gofpdf](https://github.com/go-pdf/fpdf) instead.

## Install

```bash
go get github.com/eli-eric/pdf-writer
```

The import path ends in `pdf-writer`, but the package name is `pdfwriter`,
since Go identifiers cannot contain a hyphen:

```go
import pdfwriter "github.com/eli-eric/pdf-writer"
```

Requires Go 1.21 or newer.

## Quick start

```go
package main

import (
	"log"

	pdfwriter "github.com/eli-eric/pdf-writer"
)

func main() {
	d := pdfwriter.New()
	d.AddPage()
	d.SetFont(pdfwriter.Helvetica, 12)
	d.Text(20, 20, "Hello, world")

	if err := d.Save("hello.pdf"); err != nil {
		log.Fatal(err)
	}
}
```

That produces a complete, valid PDF in **under a kilobyte**, because the
built-in fonts embed nothing.

Run the fuller demonstration, which writes a two-page `report.pdf`:

```bash
go run ./examples/report
```

## Coordinates and units

Positions are **millimetres from the top-left corner** of the page, so x grows
rightwards and y grows downwards, the way you would measure a sheet of paper.
For text, y is the baseline. Font sizes are in **points**, as is conventional
in typography.

The page is 210 × 297 mm, available as `pdfwriter.A4Width` and
`pdfwriter.A4Height`.

## Fonts

### Built-in fonts — no font file needed

`Helvetica`, `HelveticaBold`, `HelveticaOblique`, `HelveticaBoldOblique`,
`Times`, `TimesBold`, `TimesItalic`, `TimesBoldItalic`,
`Courier`, `CourierBold`, `CourierOblique`, `CourierBoldOblique`.

Every PDF viewer provides these, so nothing is embedded and the output stays a
few kilobytes.

**Their one limitation:** they can only encode WinAnsiEncoding, which covers
ASCII and Western Europe — `é`, `ü`, `ß`, `ñ`, `€` are fine. Czech, Polish,
Greek, Cyrillic and everything else are not. Rather than emit the wrong glyph
silently, the library records an error naming the character and the fix:

```
pdfwriter: the built-in font Helvetica cannot encode 'č' (U+010D); embed a
TrueType font with AddTTFFont to use this character
```

### Embedded TrueType fonts — full Unicode

```go
font, err := d.AddTTFFont("DejaVuSans", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
if err != nil {
	log.Fatal(err)
}
d.SetFont(font, 12)
d.Text(20, 20, "Příliš žluťoučký kůň úpěl ďábelské ódy.")
```

Use `AddTTFFontData` for a font you have already loaded, which is what you want
with `go:embed` — it makes the binary self-contained, with no font path to get
wrong at deployment time:

```go
//go:embed fonts/DejaVuSans.ttf
var sansTTF []byte

font, err := d.AddTTFFontData("DejaVuSans", sansTTF)
```

The font is embedded as a Type0/CIDFontType2 font with Identity-H encoding, so
there is no 256-character ceiling, and a ToUnicode map is written so the text
stays searchable and copyable.

Three things to know:

- **The whole font file is embedded, not a subset.** This is the main
  simplification the library makes. A PDF using DejaVuSans is about 390 kB
  regardless of how much text it holds; the built-in fonts cost nothing. If you
  need small files with Unicode text, you need glyph subsetting, which this
  package does not do.
- **`.ttf` only.** Fonts with PostScript outlines (usually `.otf`) and
  TrueType collections (`.ttc`) are rejected with an explanatory error.
- **Check the font's license.** Embedding a font redistributes it. See
  [NOTICE](NOTICE).

## API

| | |
|---|---|
| `New() *Doc` | new empty A4 document |
| `(*Doc) AddPage()` | append a blank page and make it current |
| `(*Doc) SetFont(f *Font, sizePt float64)` | select font and size |
| `(*Doc) SetTextColor(r, g, b uint8)` | text colour |
| `(*Doc) Text(x, y float64, s string)` | draw a line of text at a baseline |
| `(*Doc) TextRight(x, y float64, s string)` | draw text ending at x |
| `(*Doc) TextCenter(x, y float64, s string)` | draw text centred on x |
| `(*Doc) TextWidth(s string) float64` | measure text, in mm |
| `(*Doc) Paragraph(x, y, width, lineHeight float64, s string) float64` | wrapped text; returns the next baseline |
| `(*Doc) AddTTFFont(name, path string) (*Font, error)` | embed a TrueType font from disk |
| `(*Doc) AddTTFFontData(name string, data []byte) (*Font, error)` | embed a TrueType font from memory |
| `(*Doc) Save(path string) error` | write to a file |
| `(*Doc) WriteTo(w io.Writer) (int64, error)` | write to any writer |
| `(*Doc) Err() error` | first error recorded so far |
| `(*Doc) SetTitle`, `SetAuthor`, `SetCreationDate` | document metadata |
| `(*Doc) SetCompress(bool)` | Flate compression, on by default |
| `(*Doc) PageCount() int` | pages added so far |

Full documentation on
[pkg.go.dev](https://pkg.go.dev/github.com/eli-eric/pdf-writer).

### Error handling

Drawing calls do not return errors. The first failure is remembered and
returned by `Save` or `WriteTo`, so a page of layout code needs one error check
at the end rather than one per line:

```go
d.SetFont(pdfwriter.Helvetica, 11)
d.Text(20, 20, "no error check")
d.Text(20, 26, "nor here")
y := d.Paragraph(20, 40, 170, 5, body)

if err := d.Save("out.pdf"); err != nil {   // one check covers all of it
	log.Fatal(err)
}
```

`Err()` is there if you want to look sooner.

## Layout of the code

| file | contents |
|---|---|
| `doc.go` | package documentation |
| `pdf.go` | document model, object serialisation, cross-reference table |
| `text.go` | text drawing, measurement, word wrapping |
| `winansi.go` | the WinAnsiEncoding character map |
| `stdfont_metrics.go` | generated width tables for the built-in fonts |
| `ttf.go` | TrueType parsing and font embedding |
| `internal/gen` | build-time tool that generates the width tables |

About 1250 lines of library code, plus a 420-line generated table.

### Regenerating the font metrics

`stdfont_metrics.go` is generated from Adobe AFM metric files and committed, so
neither the AFMs nor the generator are needed to build or use the package. The
AFMs shipped with the URW base35 fonts are metrically identical to Adobe's
originals, so any copy of them reproduces the same table. To regenerate:

```bash
make generate AFM=/path/to/afm/dir
```

## Tests

```bash
make test
```

Beyond unit tests, the suite checks the generated files two ways: it verifies
that every cross-reference table offset really points at the object it claims
to, and — where poppler's `pdftotext` and `pdfinfo` are installed — it reads
the text and metadata back out with a parser written by someone else, which is
the only way to be confident a PDF is actually well-formed. Those tests skip
cleanly when the tools are absent, and CI installs poppler so they always run
there.

## Contributing

Bug reports and patches are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).
Please note that the narrow scope is the point: proposals that add shapes,
images or full page layout are likely to be declined in favour of a
better-suited library.

## License

MIT — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
