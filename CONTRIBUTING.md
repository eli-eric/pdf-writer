# Contributing

Thanks for your interest in pdf-writer. Bug reports, patches and documentation
fixes are all welcome.

## Scope

The narrow scope is deliberate. This package exists to write text onto A4
pages with no dependencies, and to stay small enough that one person can read
all of it. Features that would be perfectly reasonable in a general PDF
library — shapes, images, tables, multiple page sizes, page templates — are
likely to be declined here, not because they are bad ideas but because
[gopdf](https://github.com/signintech/gopdf) and
[gofpdf](https://github.com/go-pdf/fpdf) already do them well.

Changes that are very welcome:

- bugs in the generated PDF, especially anything a viewer rejects
- correctness of font metrics, encoding or text measurement
- glyph subsetting for embedded fonts, which is the one significant
  limitation the package has today
- clearer errors, documentation and examples

If you are unsure whether something fits, open an issue before writing code.

## Ground rules

**No new dependencies.** The standard library only. This is the package's main
promise and it is not negotiable. Build-time tools under `internal/` may be
more relaxed, but nothing the library imports at run time.

**Every change keeps `make check` green.** That runs formatting, `go vet`,
staticcheck and the full test suite with the race detector.

## Getting set up

```bash
git clone https://github.com/eli-eric/pdf-writer
cd pdf-writer
make check
```

Two optional tools make the tests more thorough. Both are used only for
verification, never imported:

```bash
# poppler-utils: lets the tests read text back out of generated PDFs
sudo apt install poppler-utils

# staticcheck
go install honnef.co/go/tools/cmd/staticcheck@latest
```

Tests needing poppler skip cleanly when it is absent, so `make test` works
without it — but CI installs it, so run it locally if you are touching the
writer itself.

## Testing changes to PDF output

A PDF that looks fine to the code that wrote it may still be malformed. If you
change how output is generated, verify it independently:

```bash
go run ./examples/report            # writes report.pdf
pdftotext report.pdf -              # can a real parser read the text back?
pdfinfo report.pdf                  # is the structure sound?
pdftoppm -png -r 70 report.pdf page # look at it
```

Adding a case to `pdf_test.go` that round-trips through `pdftotext` is the most
useful kind of test this package can have.

Please also confirm your change does not break the cross-reference table.
`checkStructure` in `pdf_test.go` verifies that every xref offset points at the
object it claims to; a wrong offset is the classic way a hand-written PDF
breaks, and viewers report it only as a vague error.

## Commit messages

Short imperative subject line, optionally a body explaining *why*:

```
Reject fonts with PostScript outlines

FontFile2 can only hold TrueType glyf outlines, so an .otf font produced a
PDF that every viewer rejected with an unhelpful error.
```

## Pull requests

- one logical change per pull request
- include a test for anything that was broken
- update `README.md` and the doc comments if you change the API
- add a line to `CHANGELOG.md` under "Unreleased"

By contributing you agree that your work is licensed under the MIT License, in
line with the rest of the repository.
