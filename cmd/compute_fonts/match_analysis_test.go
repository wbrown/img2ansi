package main

import (
	"testing"
)

func TestMatchAnalysis(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	tests := []struct {
		name     string
		bitmap   GlyphBitmap
		expected string // Expected character or description
	}{
		{
			name:     "SingleHorizontalLine",
			bitmap:   0x00000000FF000000, // Line at y=3
			expected: "Should match horizontal line character",
		},
		{
			name:     "SingleVerticalLine",
			bitmap:   0x0808080808080808, // Line at x=3
			expected: "Should match vertical line character",
		},
		{
			name:     "PlusSign",
			bitmap:   0x0008080808FF0808, // Cross at center
			expected: "Should match plus sign",
		},
		{
			name:     "LetterO",
			bitmap:   0x3C66C3C3C3C3663C, // Approximate O shape
			expected: "Should match letter O",
		},
		{
			name:     "Checkerboard",
			bitmap:   0xAA55AA55AA55AA55, // Alternating pixels
			expected: "Complex pattern - what matches?",
		},
		{
			name:     "HorizontalStripes",
			bitmap:   0xFF00FF00FF00FF00, // Alternating rows
			expected: "Horizontal stripes pattern",
		},
		{
			name:     "VerticalStripes", 
			bitmap:   0xCCCCCCCCCCCCCCCC, // Alternating columns
			expected: "Vertical stripes pattern",
		},
		// Diagonal Patterns
		{
			name:     "DiagonalTopLeftToBottomRight",
			bitmap:   0x8040201008040201, // Main diagonal \
			expected: "Should match backslash or similar diagonal",
		},
		{
			name:     "DiagonalTopRightToBottomLeft", 
			bitmap:   0x0102040810204080, // Anti-diagonal /
			expected: "Should match forward slash or similar diagonal",
		},
		{
			name:     "SteepDiagonal",
			bitmap:   0x8080804020201010, // Steeper angle
			expected: "Should match steep diagonal character",
		},
		// Density Variations
		{
			name:     "QuarterFill",
			bitmap:   0x000000000F0F0F0F, // Bottom half, half density
			expected: "Quarter density pattern",
		},
		{
			name:     "HalfFill",
			bitmap:   0x00000000FFFFFFFF, // Bottom half full
			expected: "Half fill pattern",
		},
		{
			name:     "ThreeQuarterFill",
			bitmap:   0xFFFFFFFF0F0F0F0F, // Top full, bottom partial
			expected: "Three quarter fill pattern",
		},
		{
			name:     "SparseDots",
			bitmap:   0x0010000010000010, // Scattered dots
			expected: "Sparse dot pattern",
		},
		// Curved and Arc Patterns
		{
			name:     "TopArc",
			bitmap:   0x1C36636363636363, // Top rounded like (
			expected: "Should match curved bracket or similar",
		},
		{
			name:     "BottomArc", 
			bitmap:   0x6363636363361C08, // Bottom rounded like )
			expected: "Should match curved bracket or similar",
		},
		{
			name:     "Circle",
			bitmap:   0x3C66C3C3C3C3663C, // Circle outline
			expected: "Should match O or circular character",
		},
		{
			name:     "QuarterCircle",
			bitmap:   0x0F1F3F7F7F3F1F0F, // Quarter circle
			expected: "Should match rounded corner character",
		},
		// Letter-like Shapes
		{
			name:     "LetterA",
			bitmap:   0x183C66667E666666, // A shape
			expected: "Should match letter A",
		},
		{
			name:     "LetterE",
			bitmap:   0x7E6060607C60607E, // E shape
			expected: "Should match letter E",
		},
		{
			name:     "LetterF",
			bitmap:   0x7E6060607C606060, // F shape
			expected: "Should match letter F",
		},
		{
			name:     "LetterL",
			bitmap:   0x606060606060607E, // L shape
			expected: "Should match letter L",
		},
		{
			name:     "LetterT",
			bitmap:   0x7E18181818181818, // T shape
			expected: "Should match letter T",
		},
		// Symmetry Tests
		{
			name:     "HorizontalSymmetry",
			bitmap:   0x183C7E7E7E3C1800, // Diamond shape
			expected: "Horizontally symmetric pattern",
		},
		{
			name:     "VerticalSymmetry",
			bitmap:   0x1818183C3C181818, // Vertical symmetric
			expected: "Vertically symmetric pattern", 
		},
		{
			name:     "RotationalSymmetry",
			bitmap:   0x003C4242423C0000, // 4-way rotational
			expected: "Rotationally symmetric pattern",
		},
		// Edge Cases
		{
			name:     "SinglePixel",
			bitmap:   0x0000000000080000, // One pixel
			expected: "Should match dot or period",
		},
		{
			name:     "TwoPixels",
			bitmap:   0x0000000000180000, // Two adjacent pixels
			expected: "Should match colon or similar",
		},
		{
			name:     "CornerPixels",
			bitmap:   0x8100000000000081, // Four corners
			expected: "Corner pattern",
		},
		{
			name:     "BorderFrame",
			bitmap:   0xFF818181818181FF, // Border outline
			expected: "Should match box drawing character",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Pattern: %s", tt.name)
			t.Logf("Expected: %s", tt.expected)
			t.Logf("Input bitmap:\n%s", tt.bitmap.String())
			
			match := lookup.FindClosestGlyph(tt.bitmap)
			t.Logf("Best match: '%c' (U+%04X)", match.Rune, match.Rune)
			t.Logf("Match bitmap:\n%s", match.Bitmap.String())
			
			// Calculate similarity details
			inputFeatures := extractFeatures(tt.bitmap)
			matchFeatures := extractFeatures(match.Bitmap)
			
			similarity := calculateSimilarity(inputFeatures, matchFeatures)
			shapeSim := calculateShapeSimilarity(inputFeatures.Bitmap, matchFeatures.Bitmap)
			patternSim := calculatePatternSimilarity(inputFeatures.Bitmap, matchFeatures.Bitmap)
			densitySim := calculateDensitySimilarity(inputFeatures.Bitmap, matchFeatures.Bitmap)
			
			t.Logf("Similarity: %.3f (Shape: %.3f, Pattern: %.3f, Density: %.3f)", 
				similarity, shapeSim, patternSim, densitySim)
			
			// Also show top 3 matches
			t.Log("\nTop 3 matches:")
			similar := calculateSimilarityBatch(inputFeatures, glyphs)
			for i := 0; i < 3 && i < len(similar); i++ {
				g := similar[i].Target
				t.Logf("%d. '%c' (U+%04X) - Similarity: %.3f", 
					i+1, g.Rune, g.Rune, similar[i].Similarity)
			}
			t.Log("")
		})
	}
}