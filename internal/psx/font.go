package psx

// WipeOut 2097's menu font, ported from maybe_BuildTextCharacterQuadPrimitive
// (SLES_003.27 0x800602e8). The glyphs live in TEXTURES/WOFONT.TIM as a grid of
// 6x6 cells, 13 per row -- 13*6 = 78 and 4*6 = 24, which is why that file is
// 80x24.
//
// The retail routine walks the string a byte at a time, maps each character to a
// glyph index, derives the cell from that index, emits a 6x6 textured quad, and
// advances the pen.

const (
	// FontCellSize is the width and height of one glyph cell in pixels.
	FontCellSize = 6
	// FontColumns is how many cells fit across the atlas.
	FontColumns = 13
	// FontAdvance is the normal pen advance, `s2 += 6`.
	FontAdvance = 6
	// FontSpaceAdvance is the advance for a space, which emits no quad: `s2 += 3`.
	FontSpaceAdvance = 3

	// FontNarrowGlyph is the one glyph retail special-cases. Index 8 is 'I', which
	// is drawn 2px left of the pen (`s2 -= 2`) and advances only 4 (`s2 += 4`).
	FontNarrowGlyph = 8
	// FontNarrowKern and FontNarrowAdvance are that glyph's offset and advance.
	FontNarrowKern    = -2
	FontNarrowAdvance = 4

	// FontMissingGlyph is the index used for any character with no mapping.
	FontMissingGlyph = 0x2a
)

// FontGlyphIndex maps a character to its cell index, reproducing retail's
// if/else chain exactly. The second return is false for a space, which advances
// the pen without drawing.
//
// The ordering is not alphabetical beyond the letters: digits sit at 29..38 via
// `c - 0x13`, and the punctuation indices are assigned individually.
func FontGlyphIndex(c byte) (int, bool) {
	switch {
	case c == ' ':
		return 0, false
	case c >= 'A' && c <= 'Z':
		return int(c) - 0x41, true
	case c >= 'a' && c <= 'z':
		// Retail only tests the uppercase range, so lowercase would fall through to
		// the missing glyph. Every string in the executable is uppercase. Folding
		// here keeps callers from having to shout, and cannot change retail output.
		return int(c) - 0x61, true
	case c >= '0' && c <= '9':
		return int(c) - 0x13, true
	case c == '.':
		return 0x1a, true
	case c == ',':
		return 0x1b, true
	case c == ':':
		return 0x1c, true
	case c == '[':
		return 0x27, true
	case c == ']':
		return 0x28, true
	case c == '/':
		return 0x29, true
	case c == '?':
		return 0x2b, true
	case c == '-':
		return 0x2c, true
	case c == '+':
		return 0x2d, true
	case c == '%':
		return 0x2e, true
	case c == '!':
		return 0x2f, true
	case c == '\'':
		// Verified against the chain at 0x800603b4: the apostrophe maps to 0x2a. That
		// is the same index this file uses for unmapped characters, so apostrophes
		// rendered correctly before this case existed -- by coincidence rather than by
		// intent, which is worth having explicit since track names contain them.
		return 0x2a, true
	}
	// Retail has no fallback here: for an unmapped character it leaves the index at
	// `c - 'A'`, which is negative for anything below 'A' and reads outside the glyph
	// set. Clamping to a printable glyph is a deliberate deviation, chosen so a stray
	// character cannot sample arbitrary texture memory.
	return FontMissingGlyph, true
}

// FontGlyphCell returns the top-left pixel of a glyph's cell in the atlas.
//
//	row = index / 13;  v = row * 6
//	col = index - row * 13;  u = col * 6
func FontGlyphCell(index int) (u, v int) {
	row := index / FontColumns
	col := index - row*FontColumns
	return col * FontCellSize, row * FontCellSize
}

// FontGlyph is one positioned glyph produced by laying out a string.
type FontGlyph struct {
	// X and Y are the top-left of the 6x6 quad in screen pixels.
	X, Y int
	// U and V are the top-left of the source cell in the atlas.
	U, V int
}

// LayoutText positions each drawable glyph of s starting at the pen (x, y),
// applying retail's space advance and narrow-glyph kern. It returns the glyphs and
// the final pen x, so callers can chain or measure.
func LayoutText(s string, x, y int) ([]FontGlyph, int) {
	glyphs := make([]FontGlyph, 0, len(s))
	for i := 0; i < len(s); i++ {
		index, draw := FontGlyphIndex(s[i])
		if !draw {
			x += FontSpaceAdvance
			continue
		}
		drawX := x
		advance := FontAdvance
		if index == FontNarrowGlyph {
			drawX += FontNarrowKern
			advance = FontNarrowAdvance
		}
		u, v := FontGlyphCell(index)
		glyphs = append(glyphs, FontGlyph{X: drawX, Y: y, U: u, V: v})
		x += advance
	}
	return glyphs, x
}

// TextWidth measures a string in pixels without allocating glyphs.
func TextWidth(s string) int {
	x := 0
	for i := 0; i < len(s); i++ {
		index, draw := FontGlyphIndex(s[i])
		if !draw {
			x += FontSpaceAdvance
			continue
		}
		if index == FontNarrowGlyph {
			x += FontNarrowAdvance
			continue
		}
		x += FontAdvance
	}
	return x
}
