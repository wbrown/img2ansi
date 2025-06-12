package img2ansi

import (
	"testing"
)

// TestBitwiseBlockOrdering documents the historical bitwise bug and verifies
// that the blocks array is correctly ordered to make the bitwise method work.
//
// Background: The error diffusion code uses a bitwise operation on array indices
// to determine which quadrants should use foreground vs background color:
//
//	if (runeIndex & (1 << (3 - i))) != 0
//
// This only works if the blocks array is ordered so that each index (0-15)
// encodes the quadrant pattern in binary:
//
//	Bit 3: Top-left
//	Bit 2: Top-right
//	Bit 1: Bottom-left
//	Bit 0: Bottom-right
//
// For example, index 5 (0b0101) should map to a character with
// top-right and bottom-right quadrants filled.
func TestBitwiseBlockOrdering(t *testing.T) {
	t.Log("Verifying blocks array is correctly ordered for bitwise operations")

	// Verify each block's quadrants match its index's bit pattern
	for idx, block := range blocks {
		// Extract expected quadrants from index bits
		expectedTL := (idx & 0b1000) != 0
		expectedTR := (idx & 0b0100) != 0
		expectedBL := (idx & 0b0010) != 0
		expectedBR := (idx & 0b0001) != 0

		// Check against actual quadrants
		if block.Quad.TopLeft != expectedTL ||
			block.Quad.TopRight != expectedTR ||
			block.Quad.BottomLeft != expectedBL ||
			block.Quad.BottomRight != expectedBR {
			t.Errorf("Block at index %d (%04b) has incorrect quadrants:\n"+
				"  Character: %c\n"+
				"  Expected: TL=%v TR=%v BL=%v BR=%v\n"+
				"  Actual:   TL=%v TR=%v BL=%v BR=%v",
				idx, idx, block.Rune,
				expectedTL, expectedTR, expectedBL, expectedBR,
				block.Quad.TopLeft, block.Quad.TopRight,
				block.Quad.BottomLeft, block.Quad.BottomRight)
		}
	}

	// Verify critical characters are at correct positions
	criticalChars := map[int]rune{
		0:  ' ', // Empty (0b0000)
		15: '█', // Full (0b1111)
	}

	for idx, expectedRune := range criticalChars {
		if blocks[idx].Rune != expectedRune {
			t.Errorf("Critical character at index %d should be '%c' but is '%c'",
				idx, expectedRune, blocks[idx].Rune)
		}
	}

	t.Log("✓ Blocks array is correctly ordered for bitwise operations")
}

// TestErrorDiffusionBitwiseMethod verifies that error diffusion works correctly
// with the bitwise method after the block reordering fix.
func TestErrorDiffusionBitwiseMethod(t *testing.T) {
	// This test verifies that the bitwise operation in error diffusion
	// correctly identifies which quadrants to use for error calculation

	testRunes := []rune{'▘', '▀', '▌', '█'} // Various patterns

	for _, testRune := range testRunes {
		// Find the index
		var runeIndex int
		found := false
		for idx, b := range blocks {
			if b.Rune == testRune {
				runeIndex = idx
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Test rune '%c' not found in blocks array", testRune)
			continue
		}

		// Get expected quadrants from the blocks array
		var expected Quadrants
		for _, b := range blocks {
			if b.Rune == testRune {
				expected = b.Quad
				break
			}
		}

		// Verify bitwise operation gives correct results
		bitwiseTL := (runeIndex & (1 << 3)) != 0
		bitwiseTR := (runeIndex & (1 << 2)) != 0
		bitwiseBL := (runeIndex & (1 << 1)) != 0
		bitwiseBR := (runeIndex & (1 << 0)) != 0

		if bitwiseTL != expected.TopLeft ||
			bitwiseTR != expected.TopRight ||
			bitwiseBL != expected.BottomLeft ||
			bitwiseBR != expected.BottomRight {
			t.Errorf("Bitwise operation fails for rune '%c' at index %d:\n"+
				"  Bitwise: TL=%v TR=%v BL=%v BR=%v\n"+
				"  Expected: TL=%v TR=%v BL=%v BR=%v",
				testRune, runeIndex,
				bitwiseTL, bitwiseTR, bitwiseBL, bitwiseBR,
				expected.TopLeft, expected.TopRight, expected.BottomLeft, expected.BottomRight)
		}
	}
}
