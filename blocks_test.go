package img2ansi

import "testing"

// TestBlocksIndexEncodesQuadrants verifies the documented invariant that
// each block's index in the Blocks array encodes its quadrant pattern:
// bit 0 = top-left, bit 1 = top-right, bit 2 = bottom-left,
// bit 3 = bottom-right.
//
// Note this is a property of the array INDEX, not the rune codepoint.
// An earlier version of the error diffusion code tested codepoint bits,
// which disagrees with the quadrant table for 25 of 64 pixels; diffusion
// now uses getQuadrantsForRune.
func TestBlocksIndexEncodesQuadrants(t *testing.T) {
	if len(Blocks) != 16 {
		t.Fatalf("Blocks should have 16 entries, got %d", len(Blocks))
	}
	for idx, b := range Blocks {
		want := Quadrants{
			TopLeft:     idx&1 != 0,
			TopRight:    idx&2 != 0,
			BottomLeft:  idx&4 != 0,
			BottomRight: idx&8 != 0,
		}
		if b.Quad != want {
			t.Errorf("Blocks[%d] (%q): quadrants %+v do not match index bits %04b",
				idx, b.Rune, b.Quad, idx)
		}
	}
}

// TestRuneQuadrantsMatchesBlocks verifies the diffusion lookup map agrees
// with the Blocks table for every block character.
func TestRuneQuadrantsMatchesBlocks(t *testing.T) {
	for _, b := range Blocks {
		if got := getQuadrantsForRune(b.Rune); got != b.Quad {
			t.Errorf("getQuadrantsForRune(%q) = %+v, want %+v",
				b.Rune, got, b.Quad)
		}
	}
	if got := getQuadrantsForRune('x'); got != (Quadrants{}) {
		t.Errorf("getQuadrantsForRune of a non-block rune should be empty, got %+v", got)
	}
}
