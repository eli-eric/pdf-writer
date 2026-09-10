package pdfwriter

// winAnsiHigh maps the runes that WinAnsiEncoding places in the 0x80-0x9F
// range to their byte codes. Every other printable WinAnsi code equals its
// Unicode code point: 0x20-0x7E is ASCII and 0xA0-0xFF is Latin-1.
var winAnsiHigh = map[rune]byte{
	'€': 0x80, // Euro
	'‚': 0x82, // quotesinglbase
	'ƒ': 0x83, // florin
	'„': 0x84, // quotedblbase
	'…': 0x85, // ellipsis
	'†': 0x86, // dagger
	'‡': 0x87, // daggerdbl
	'ˆ': 0x88, // circumflex
	'‰': 0x89, // perthousand
	'Š': 0x8A, // Scaron
	'‹': 0x8B, // guilsinglleft
	'Œ': 0x8C, // OE
	'Ž': 0x8E, // Zcaron
	'‘': 0x91, // quoteleft
	'’': 0x92, // quoteright
	'“': 0x93, // quotedblleft
	'”': 0x94, // quotedblright
	'•': 0x95, // bullet
	'–': 0x96, // endash
	'—': 0x97, // emdash
	'˜': 0x98, // tilde
	'™': 0x99, // trademark
	'š': 0x9A, // scaron
	'›': 0x9B, // guilsinglright
	'œ': 0x9C, // oe
	'ž': 0x9E, // zcaron
	'Ÿ': 0x9F, // Ydieresis
}

// winAnsiEncode converts r to its WinAnsiEncoding byte. The second result is
// false when the encoding cannot represent r at all, which is the case for
// every script outside Western Europe -- including Czech letters such as
// c-caron and r-caron. Embed a TrueType font with [Doc.AddTTFFont] to write
// those.
func winAnsiEncode(r rune) (byte, bool) {
	switch {
	case r >= 0x20 && r <= 0x7E:
		return byte(r), true
	case r >= 0xA0 && r <= 0xFF:
		return byte(r), true
	}
	b, ok := winAnsiHigh[r]
	return b, ok
}
