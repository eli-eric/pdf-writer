// Command gen generates stdfont_metrics.go from Adobe AFM metric files.
//
// The AFMs shipped with the URW base35 fonts are metrically identical to
// Adobe's originals, so any copy of them produces the same tables. Run:
//
//	go run ./internal/gen -afm /path/to/afm/dir > stdfont_metrics.go
//
// This program is a build-time tool only; the generated table is committed,
// so the library itself needs neither the AFMs nor this program.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

// afmFiles maps a PDF base font name to its URW AFM file basename.
var afmFiles = [][2]string{
	{"Helvetica", "phvr8a"},
	{"Helvetica-Bold", "phvb8a"},
	{"Helvetica-Oblique", "phvro8a"},
	{"Helvetica-BoldOblique", "phvbo8a"},
	{"Times-Roman", "ptmr8a"},
	{"Times-Bold", "ptmb8a"},
	{"Times-Italic", "ptmri8a"},
	{"Times-BoldItalic", "ptmbi8a"},
	{"Courier", "pcrr8a"},
	{"Courier-Bold", "pcrb8a"},
	{"Courier-Oblique", "pcrro8a"},
	{"Courier-BoldOblique", "pcrbo8a"},
}

// winAnsiNames lists the Adobe glyph name for each WinAnsiEncoding code from
// 32 to 255. An empty string marks a code left undefined by the encoding.
// Source: PDF 1.7 specification, Annex D.2.
var winAnsiNames = [224]string{
	// 32..47
	"space", "exclam", "quotedbl", "numbersign", "dollar", "percent",
	"ampersand", "quotesingle", "parenleft", "parenright", "asterisk",
	"plus", "comma", "hyphen", "period", "slash",
	// 48..63
	"zero", "one", "two", "three", "four", "five", "six", "seven", "eight",
	"nine", "colon", "semicolon", "less", "equal", "greater", "question",
	// 64..79
	"at", "A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
	"N", "O",
	// 80..95
	"P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z", "bracketleft",
	"backslash", "bracketright", "asciicircum", "underscore",
	// 96..111
	"grave", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l",
	"m", "n", "o",
	// 112..127
	"p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z", "braceleft",
	"bar", "braceright", "asciitilde", "",
	// 128..143
	"Euro", "", "quotesinglbase", "florin", "quotedblbase", "ellipsis",
	"dagger", "daggerdbl", "circumflex", "perthousand", "Scaron",
	"guilsinglleft", "OE", "", "Zcaron", "",
	// 144..159
	"", "quoteleft", "quoteright", "quotedblleft", "quotedblright",
	"bullet", "endash", "emdash", "tilde", "trademark", "scaron",
	"guilsinglright", "oe", "", "zcaron", "Ydieresis",
	// 160..175
	"space", "exclamdown", "cent", "sterling", "currency", "yen",
	"brokenbar", "section", "dieresis", "copyright", "ordfeminine",
	"guillemotleft", "logicalnot", "hyphen", "registered", "macron",
	// 176..191
	"degree", "plusminus", "twosuperior", "threesuperior", "acute", "mu",
	"paragraph", "periodcentered", "cedilla", "onesuperior",
	"ordmasculine", "guillemotright", "onequarter", "onehalf",
	"threequarters", "questiondown",
	// 192..207
	"Agrave", "Aacute", "Acircumflex", "Atilde", "Adieresis", "Aring",
	"AE", "Ccedilla", "Egrave", "Eacute", "Ecircumflex", "Edieresis",
	"Igrave", "Iacute", "Icircumflex", "Idieresis",
	// 208..223
	"Eth", "Ntilde", "Ograve", "Oacute", "Ocircumflex", "Otilde",
	"Odieresis", "multiply", "Oslash", "Ugrave", "Uacute", "Ucircumflex",
	"Udieresis", "Yacute", "Thorn", "germandbls",
	// 224..239
	"agrave", "aacute", "acircumflex", "atilde", "adieresis", "aring",
	"ae", "ccedilla", "egrave", "eacute", "ecircumflex", "edieresis",
	"igrave", "iacute", "icircumflex", "idieresis",
	// 240..255
	"eth", "ntilde", "ograve", "oacute", "ocircumflex", "otilde",
	"odieresis", "divide", "oslash", "ugrave", "uacute", "ucircumflex",
	"udieresis", "yacute", "thorn", "ydieresis",
}

// fallbackWidths supplies metrics for glyphs that WinAnsiEncoding defines but
// that the URW AFMs omit. Values are Adobe's published widths.
var fallbackWidths = map[string]map[string]int{
	"Euro": {
		"Helvetica": 556, "Helvetica-Bold": 556,
		"Helvetica-Oblique": 556, "Helvetica-BoldOblique": 556,
		"Times-Roman": 500, "Times-Bold": 500,
		"Times-Italic": 500, "Times-BoldItalic": 500,
		"Courier": 600, "Courier-Bold": 600,
		"Courier-Oblique": 600, "Courier-BoldOblique": 600,
	},
}

// parseAFM returns the advance width of every named glyph in an AFM file.
func parseAFM(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	widths := make(map[string]int)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "C ") {
			continue
		}
		// Character metric lines look like:
		//   C 32 ; WX 278 ; N space ; B 0 0 0 0 ;
		var wx int
		var name string
		haveWX := false
		for _, field := range strings.Split(line, ";") {
			parts := strings.Fields(field)
			if len(parts) < 2 {
				continue
			}
			switch parts[0] {
			case "WX":
				if wx, err = strconv.Atoi(parts[1]); err != nil {
					return nil, fmt.Errorf("%s: bad WX %q", path, parts[1])
				}
				haveWX = true
			case "N":
				name = parts[1]
			}
		}
		if haveWX && name != "" {
			widths[name] = wx
		}
	}
	return widths, sc.Err()
}

func main() {
	dir := flag.String("afm", "", "directory holding the URW base35 AFM files")
	flag.Parse()
	if *dir == "" {
		log.Fatal("-afm is required")
	}

	var out strings.Builder
	out.WriteString("// Code generated by internal/gen from Adobe AFM metrics. DO NOT EDIT.\n\n")
	out.WriteString("package pdfwriter\n\n")
	out.WriteString("// stdWidths holds the advance width of every WinAnsiEncoding code for each\n")
	out.WriteString("// of the 12 text fonts built into every PDF viewer. Widths are in glyph\n")
	out.WriteString("// space units, i.e. thousandths of the font size. Codes the encoding\n")
	out.WriteString("// leaves undefined are zero.\n")
	out.WriteString("var stdWidths = map[string]*[256]uint16{\n")

	missing := map[string][]string{}
	for _, ff := range afmFiles {
		name, file := ff[0], ff[1]
		widths, err := parseAFM(*dir + "/" + file + ".afm")
		if err != nil {
			log.Fatal(err)
		}

		var table [256]uint16
		for i, glyph := range winAnsiNames {
			if glyph == "" {
				continue
			}
			code := i + 32
			w, ok := widths[glyph]
			if !ok {
				if fb, hit := fallbackWidths[glyph][name]; hit {
					w = fb
				} else {
					missing[name] = append(missing[name], glyph)
					continue
				}
			}
			table[code] = uint16(w)
		}

		fmt.Fprintf(&out, "\t%q: {\n", name)
		for row := 0; row < 256; row += 8 {
			out.WriteString("\t\t")
			for c := row; c < row+8; c++ {
				fmt.Fprintf(&out, "%d, ", table[c])
			}
			fmt.Fprintf(&out, "// %d-%d\n", row, row+7)
		}
		out.WriteString("\t},\n")
	}
	out.WriteString("}\n")

	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for k := range missing {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(os.Stderr, "warning: %s missing %v\n", k, missing[k])
		}
	}

	os.Stdout.WriteString(out.String())
}
