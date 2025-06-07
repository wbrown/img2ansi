package main

import (
	"strings"
	"testing"
)

func TestSpecificMatches(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	tests := []struct {
		name     string
		pattern  string
		expected rune
	}{
		{
			name: "Horizontal Line",
			pattern: `
········
········
········
██████··
········
········
········
········`,
			expected: '-',
		},
		{
			name: "Vertical Line", 
			pattern: `
···██···
···██···
···██···
········
···██···
···██···
···██···
········`,
			expected: '|',
		},
		{
			name: "Plus Sign",
			pattern: `
········
··██····
··██····
██████··
··██····
··██····
········
········`,
			expected: '+',
		},
		{
			name: "Letter O",
			pattern: `
··███···
·██·██··
██···██·
██···██·
██···██·
·██·██··
··███···
········`,
			expected: 'O',
		},
		{
			name: "Larger Circle",
			pattern: `
··████··
·██··██·
██····██
██····██
██····██
██····██
·██··██·
··████··`,
			expected: 'O', // Should still match O even with different size
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bitmap := parseBitmap(tt.pattern)
			match := lookup.FindClosestGlyph(bitmap)
			
			t.Logf("Input pattern:\n%s", bitmap.String())
			t.Logf("Matched: '%c' (U+%04X)", match.Rune, match.Rune)
			t.Logf("Match pattern:\n%s", match.Bitmap.String())
			
			// Show similarity details
			inputFeatures := extractFeatures(bitmap)
			matchFeatures := extractFeatures(match.Bitmap)
			
			similarity := calculateSimilarity(inputFeatures, matchFeatures)
			shapeSim := calculateShapeSimilarity(inputFeatures.Bitmap, matchFeatures.Bitmap)
			patternSim := calculatePatternSimilarity(inputFeatures.Bitmap, matchFeatures.Bitmap)
			densitySim := calculateDensitySimilarity(inputFeatures.Bitmap, matchFeatures.Bitmap)
			
			t.Logf("Total Similarity: %.3f", similarity)
			t.Logf("Shape: %.3f, Pattern: %.3f, Density: %.3f", shapeSim, patternSim, densitySim)
			
			if match.Rune != tt.expected {
				t.Errorf("Expected '%c' but got '%c'", tt.expected, match.Rune)
			}
		})
	}
}

// Helper to parse visual bitmap strings
func parseBitmap(s string) GlyphBitmap {
	var bitmap GlyphBitmap
	lines := strings.Split(strings.TrimSpace(s), "\n")
	
	// Parse each position
	for y := 0; y < 8 && y < len(lines); y++ {
		runes := []rune(lines[y])
		for x := 0; x < 8 && x < len(runes); x++ {
			if runes[x] == '█' || runes[x] == '*' {
				bitPos := y*8 + x
				bitmap = bitmap | (1 << bitPos)
			}
		}
	}
	
	return bitmap
}