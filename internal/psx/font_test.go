package psx

import (
	"os"
	"testing"
)

// The mapping is retail's if/else chain in maybe_BuildTextCharacterQuadPrimitive.
// Digits are the non-obvious one: `c - 0x13` puts '0' at 29, not adjacent to 'Z'.
func TestFontGlyphIndexMapping(t *testing.T) {
	for _, tc := range []struct {
		c    byte
		want int
	}{
		{'A', 0}, {'I', 8}, {'Z', 25},
		{'.', 0x1a}, {',', 0x1b}, {':', 0x1c},
		{'0', 0x1d}, {'9', 0x26},
		{'[', 0x27}, {']', 0x28}, {'/', 0x29},
		{'?', 0x2b}, {'-', 0x2c}, {'+', 0x2d}, {'%', 0x2e}, {'!', 0x2f},
		{'#', FontMissingGlyph}, {'@', FontMissingGlyph},
	} {
		got, draw := FontGlyphIndex(tc.c)
		if !draw {
			t.Errorf("%q reported as non-drawing", tc.c)
			continue
		}
		if got != tc.want {
			t.Errorf("FontGlyphIndex(%q) = %#x, want %#x", tc.c, got, tc.want)
		}
	}
	if _, draw := FontGlyphIndex(' '); draw {
		t.Error("space must not draw a quad")
	}
}

// Digits must land on the row the cell arithmetic implies, since an off-by-one here
// would show as the wrong character rather than a crash.
func TestFontDigitsAreContiguous(t *testing.T) {
	for d := byte('0'); d <= '9'; d++ {
		idx, _ := FontGlyphIndex(d)
		want := 0x1d + int(d-'0')
		if idx != want {
			t.Errorf("digit %q index %#x, want %#x", d, idx, want)
		}
	}
}

func TestFontGlyphCell(t *testing.T) {
	// Index 0 is the origin, 12 is the last cell of row 0, 13 starts row 1.
	for _, tc := range []struct {
		index, u, v int
	}{
		{0, 0, 0},
		{12, 72, 0},
		{13, 0, 6},
		{25, 72, 6},
		{26, 0, 12},
		{FontMissingGlyph, (0x2a - 3*13) * 6, 18},
	} {
		u, v := FontGlyphCell(tc.index)
		if u != tc.u || v != tc.v {
			t.Errorf("cell(%d) = (%d,%d), want (%d,%d)", tc.index, u, v, tc.u, tc.v)
		}
	}
}

// Retail draws 'I' two pixels left of the pen and advances only four.
func TestFontNarrowGlyphKern(t *testing.T) {
	glyphs, pen := LayoutText("II", 100, 50)
	if len(glyphs) != 2 {
		t.Fatalf("laid out %d glyphs, want 2", len(glyphs))
	}
	if glyphs[0].X != 98 {
		t.Errorf("first I drawn at x=%d, want 98 (pen 100 kerned by -2)", glyphs[0].X)
	}
	if glyphs[1].X != 102 {
		t.Errorf("second I drawn at x=%d, want 102", glyphs[1].X)
	}
	if pen != 108 {
		t.Errorf("pen ended at %d, want 108 (two advances of 4)", pen)
	}
}

// A space emits no glyph but still moves the pen, by 3 rather than 6.
func TestFontSpaceAdvance(t *testing.T) {
	glyphs, pen := LayoutText("A A", 0, 0)
	if len(glyphs) != 2 {
		t.Fatalf("laid out %d glyphs, want 2 (the space draws nothing)", len(glyphs))
	}
	if glyphs[1].X != FontAdvance+FontSpaceAdvance {
		t.Errorf("second A at x=%d, want %d", glyphs[1].X, FontAdvance+FontSpaceAdvance)
	}
	if pen != 2*FontAdvance+FontSpaceAdvance {
		t.Errorf("pen at %d, want %d", pen, 2*FontAdvance+FontSpaceAdvance)
	}
}

func TestTextWidthMatchesLayout(t *testing.T) {
	for _, s := range []string{"PRESS START", "CD TRACK: 6", "TIN THERE [EDIT]", "III", ""} {
		_, pen := LayoutText(s, 0, 0)
		if got := TextWidth(s); got != pen {
			t.Errorf("TextWidth(%q) = %d but layout ended at %d", s, got, pen)
		}
	}
}

// The grid geometry is only correct if it matches the actual atlas. 13 columns of
// 6px is 78, and the glyph set needs 4 rows, so the file must be at least 78x24 --
// WOFONT.TIM is 80x24.
func TestFontAtlasFitsTheGridAssumption(t *testing.T) {
	data, err := os.ReadFile("/home/epkcfsm/src/vibeout-go/assets/WIPEOUT2/TEXTURES/WOFONT.TIM")
	if err != nil {
		t.Skip(err)
	}
	img, err := DecodeTIM(data)
	if err != nil {
		t.Fatal(err)
	}
	needW := FontColumns * FontCellSize
	rows := (FontMissingGlyph + FontColumns) / FontColumns
	needH := rows * FontCellSize
	if img.Width < needW || img.Height < needH {
		t.Errorf("atlas is %dx%d but the grid needs at least %dx%d",
			img.Width, img.Height, needW, needH)
	}
	// Every mapped glyph must fall inside the atlas.
	for c := byte(0x20); c < 0x7f; c++ {
		idx, draw := FontGlyphIndex(c)
		if !draw {
			continue
		}
		u, v := FontGlyphCell(idx)
		if u+FontCellSize > img.Width || v+FontCellSize > img.Height {
			t.Errorf("glyph %q (index %#x) cell (%d,%d) falls outside the %dx%d atlas",
				c, idx, u, v, img.Width, img.Height)
		}
	}
}

// The apostrophe maps to 0x2a, verified at 0x800603b4. Track names contain them, and
// this happened to work before the case existed because 0x2a is also the index used
// for unmapped characters -- coincidence, not intent.
func TestFontApostrophe(t *testing.T) {
	index, draw := FontGlyphIndex('\'')
	if !draw {
		t.Fatal("the apostrophe must draw")
	}
	if index != 0x2a {
		t.Errorf("apostrophe maps to %#x, want 0x2a", index)
	}
	// It must lay out inside a real name.
	glyphs, width := LayoutText("TALON'S REACH", 0, 0)
	if len(glyphs) != 12 {
		t.Errorf("laid out %d glyphs for TALON'S REACH, want 12 (the space draws nothing)", len(glyphs))
	}
	if width <= 0 {
		t.Error("no width")
	}
}
