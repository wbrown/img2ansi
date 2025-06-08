package main

import (
	"fmt"
	"github.com/golang/freetype/truetype"
	"testing"
)

// LoadFonts loads the test fonts
func LoadFonts(t *testing.T) (*truetype.Font, *truetype.Font) {
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	safeFont, err := loadFont("FSD - PragmataProMono.ttf")
	if err != nil {
		t.Fatalf("Failed to load font: %v", err)
	}
	return font, safeFont
}

// TestFontCapabilities tests font capability detection
func TestFontCapabilities(t *testing.T) {
	// Test IBM BIOS font
	IBMFont, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Fatalf("Failed to load IBM BIOS font: %v", err)
	}
	
	caps := AnalyzeFontCapabilities(IBMFont, "IBM BIOS")
	
	fmt.Printf("Font: %s\n", caps.Name)
	fmt.Printf("Block Support: %v\n", caps.HasBlocks)
	fmt.Printf("Has Shading: %v\n", caps.HasShading)
	fmt.Printf("Has Box Drawing: %v\n", caps.HasBoxDrawing)
	fmt.Printf("Total Characters: %d\n", len(caps.CharacterSet))
	
	// Show available pattern characters
	patterns := caps.GetPatternCharacters()
	fmt.Printf("\nPattern Characters (%d):\n", len(patterns))
	for _, r := range patterns {
		fmt.Printf("  U+%04X %c\n", r, r)
	}
	
	// Check specific limitations
	fmt.Println("\nChecking 2x2 quadrant blocks:")
	for _, r := range quarterBlockChars {
		if IBMFont.Index(r) != 0 {
			fmt.Printf("  Has U+%04X %c\n", r, r)
		} else {
			fmt.Printf("  Missing U+%04X\n", r)
		}
	}
	
	// Verify what we learned from documentation
	if caps.HasBlocks != HalfBlocks {
		t.Errorf("Expected HalfBlocks support, got %v", caps.HasBlocks)
	}
	
	if !caps.HasShading {
		t.Error("Expected shading support")
	}
}

// TestSpecificCharacters examines specific character glyphs
func TestSpecificCharacters(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)
	
	// Find specific characters
	chars := []rune{'\\', '/', '-', '|', '+', 'O', '#'}
	
	for _, ch := range chars {
		for _, g := range glyphs {
			if g.Rune == ch {
				t.Logf("Character '%c' (U+%04X):", ch, ch)
				t.Logf("Weight: %d", g.Weight)
				t.Logf("Bitmap:\n%s", g.Bitmap.String())
				break
			}
		}
	}
}

// TestSpecificCharacterCheck verifies character availability and appearance
func TestSpecificCharacterCheck(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)
	
	// Check specific characters we expect to match
	checkChars := []struct {
		char     rune
		expected string
	}{
		{'|', "Vertical bar"},
		{'+', "Plus sign"},
		{'O', "Letter O"},
		{'-', "Hyphen/minus"},
		{'\\', "Backslash"},
		{'/', "Forward slash"},
		{'#', "Hash/pound"},
		{'█', "Full block"},
		{'·', "Middle dot"},
	}
	
	for _, cc := range checkChars {
		found := false
		for _, g := range glyphs {
			if g.Rune == cc.char {
				found = true
				t.Logf("\n%s '%c' (U+%04X):", cc.expected, cc.char, cc.char)
				t.Logf("Bitmap:\n%s", g.Bitmap.String())
				t.Logf("Weight: %d", g.Weight)
				
				// Just show the glyph info
				break
			}
		}
		if !found {
			t.Logf("\n%s '%c' (U+%04X): NOT FOUND in font analysis", cc.expected, cc.char, cc.char)
		}
	}
	
	// Also check what the actual matches were
	t.Log("\n\nChecking what these problematic matches actually are:")
	problemChars := []rune{'ι', 'τ', '½', '&'}
	for _, ch := range problemChars {
		for _, g := range glyphs {
			if g.Rune == ch {
				t.Logf("\nCharacter '%c' (U+%04X):", ch, ch)
				t.Logf("Bitmap:\n%s", g.Bitmap.String())
				break
			}
		}
	}
}

// TestFontAwareMatching tests matching with actual font character patterns
func TestFontAwareMatching(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	// Test with patterns that match the actual font characters
	tests := []struct {
		name     string
		bitmap   GlyphBitmap
		expected rune
	}{
		{
			name: "ActualVerticalBar",
			// Match the actual '|' character with gap at row 3
			bitmap: func() GlyphBitmap {
				var b GlyphBitmap
				for y := 0; y < 8; y++ {
					if y != 3 && y != 7 { // Skip rows 3 and 7
						b |= 1 << (y*8 + 3) // Set x=3
					}
				}
				return b
			}(),
			expected: '|',
		},
		{
			name: "ActualPlusSign",
			// Match the actual '+' character
			bitmap: func() GlyphBitmap {
				var b GlyphBitmap
				// Vertical part
				for y := 1; y <= 5; y++ {
					b |= 1 << (y*8 + 2) // x=2
				}
				// Horizontal part
				for x := 0; x < 6; x++ {
					b |= 1 << (3*8 + x) // y=3
				}
				return b
			}(),
			expected: '+',
		},
		{
			name: "ActualLetterO",
			// Match the actual 'O' character (7 rows)
			bitmap: func() GlyphBitmap {
				var b GlyphBitmap
				// Top row
				b |= 0x38 << (0*8) // ··███···
				// Middle rows
				b |= 0x6C << (1*8) // ·██·██··
				b |= 0xC6 << (2*8) // ██···██·
				b |= 0xC6 << (3*8) // ██···██·
				b |= 0xC6 << (4*8) // ██···██·
				b |= 0x6C << (5*8) // ·██·██··
				b |= 0x38 << (6*8) // ··███···
				return b
			}(),
			expected: 'O',
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.name)
			t.Logf("Expected: '%c' (U+%04X)", tt.expected, tt.expected)
			t.Logf("Input bitmap:\n%s", tt.bitmap.String())
			
			match := lookup.FindClosestGlyph(tt.bitmap)
			t.Logf("Best match: '%c' (U+%04X)", match.Rune, match.Rune)
			t.Logf("Match bitmap:\n%s", match.Bitmap.String())
			
			if match.Rune != tt.expected {
				t.Errorf("Expected '%c' but got '%c'", tt.expected, match.Rune)
			}
			
			// Calculate similarity
			inputFeatures := extractFeatures(tt.bitmap)
			matchFeatures := extractFeatures(match.Bitmap)
			similarity := calculateSimilarity(inputFeatures, matchFeatures)
			t.Logf("Similarity: %.3f", similarity)
		})
	}
}

// Test bitmap patterns from fonts_test.go
var testBitmaps = []struct {
	name        string
	description string
	bitmap      GlyphBitmap
}{
	{
		name:        "Solid",
		description: "Completely filled bitmap",
		bitmap:      0xFFFFFFFFFFFFFFFF,
	},
	{
		name:        "Empty",
		description: "Completely empty bitmap",
		bitmap:      0x0000000000000000,
	},
	{
		name:        "Checkerboard",
		description: "Alternating pattern",
		bitmap:      0xAA55AA55AA55AA55,
	},
	{
		name:        "VerticalLines",
		description: "Vertical stripes",
		bitmap:      0xCCCCCCCCCCCCCCCC,
	},
	{
		name:        "HorizontalLines",
		description: "Horizontal stripes",
		bitmap:      0xFF00FF00FF00FF00,
	},
	{
		name:        "DiagonalLine",
		description: "Diagonal line from top-left to bottom-right",
		bitmap:      0x8040201008040201,
	},
	{
		name:        "ReverseDiagonalLine",
		description: "Diagonal line from top-right to bottom-left",
		bitmap:      0x0102040810204080,
	},
	{
		name:        "Cross",
		description: "Cross pattern",
		bitmap:      0x1818181818181818,
	},
	{
		name:        "Frame",
		description: "Border frame",
		bitmap:      0xFF818181818181FF,
	},
	{
		name:        "Circle",
		description: "Rough circle shape",
		bitmap:      0x3C7EFFFFFFFF7E3C,
	},
	{
		name:        "UpperLeftQuarter",
		description: "Upper left quarter filled",
		bitmap:      0xF0F0F0F000000000,
	},
	{
		name:        "LowerRightQuarter",
		description: "Lower right quarter filled",
		bitmap:      0x000000000F0F0F0F,
	},
	{
		name:        "Random50Percent",
		description: "Random 50% fill",
		bitmap:      0x1A2B3C4D5E6F7A8B,
	},
	{
		name:        "Random25Percent",
		description: "Random 25% fill",
		bitmap:      0x1020304010203040,
	},
	{
		name:        "Random75Percent",
		description: "Random 75% fill",
		bitmap:      0xEFDFCFBFAF9F8F7F,
	},
	{
		name:        "VerticalGradient",
		description: "Vertical gradient",
		bitmap:      0xFFEEDDCCBBAA9988,
	},
	{
		name:        "HorizontalGradient",
		description: "Horizontal gradient",
		bitmap:      0x8040201008040201,
	},
	{
		name:        "CentralDot",
		description: "Single dot in the center",
		bitmap:      0x0000001818000000,
	},
	{
		name:        "FourCorners",
		description: "Dots in four corners",
		bitmap:      0x8100000000000081,
	},
	{
		name:        "DiamondShape",
		description: "Diamond shape",
		bitmap:      0x183C7EFFFF7E3C18,
	},
}

func ShowSimilarity(t *testing.T, a GlyphBitmap, b GlyphBitmap) {
	aFeatures := extractFeatures(a)
	bFeatures := extractFeatures(b)

	t.Logf("Similarity: %f\n", calculateSimilarity(aFeatures, bFeatures))
	t.Logf("Shape Similarity: %f\n", calculateShapeSimilarity(aFeatures.Bitmap, bFeatures.Bitmap))
	t.Logf("Pattern Similarity: %f\n", calculatePatternSimilarity(aFeatures.Bitmap, bFeatures.Bitmap))
	t.Logf("Density Similarity: %f\n", calculateDensitySimilarity(aFeatures.Bitmap, bFeatures.Bitmap))
}

func TestDiagonalMatching(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)

	glyphLookup := NewGlyphLookup(glyphs)
	diagonalLineRune := '\\'
	reverseDiagonalLineRune := '/'

	// Assuming you have a slice of GlyphInfo called `glyphs`
	for _, test := range testBitmaps {
		t.Run(test.name, func(t *testing.T) {
			if test.name != "DiagonalLine" && test.name != "ReverseDiagonalLine" {
				return
			}
			var expectedRune rune
			if test.name == "DiagonalLine" {
				expectedRune = diagonalLineRune
			} else {
				expectedRune = reverseDiagonalLineRune
			}
			lookUp := glyphLookup.LookupRune(expectedRune)
			if lookUp == nil {
				t.Fatalf("Failed to find rune %c in the font", expectedRune)
			}
			testFeatures := extractFeatures(test.bitmap)
			forcedSimilarityScore := calculateSimilarity(testFeatures, *lookUp)
			similiar := calculateSimilarityBatch(testFeatures, glyphs)

			t.Logf("Test: %s\nDescription: %s\nInput:\n%s\nForced Match:\n%s\nSimilarity: %f\n",
				test.name,
				test.description,
				test.bitmap.String(),
				lookUp.Bitmap.String(),
				forcedSimilarityScore)
			ShowSimilarity(t, test.bitmap, lookUp.Bitmap)

			for idx := range similiar {
				glyph := similiar[idx].Target
				t.Logf("Similarity: %f\n%s\n", similiar[idx].Similarity, glyph.Bitmap.String())
				ShowSimilarity(t, test.bitmap, glyph.Bitmap)
				if idx > 10 {
					break
				}
			}
		})
	}
}

func TestGlyphMatching(t *testing.T) {
	font, safeFont := LoadFonts(t)
	glyphs := analyzeFont(font, safeFont)

	glyphLookup := NewGlyphLookup(glyphs)

	// Assuming you have a slice of GlyphInfo called `glyphs`
	for _, test := range testBitmaps {
		t.Run(test.name, func(t *testing.T) {
			closestGlyph := glyphLookup.FindClosestGlyph(test.bitmap)
			t.Logf("Test: %s\nDescription: %s\nInput:\n%s\nClosest Match:\n%s\n",
				test.name, test.description, test.bitmap.String(), closestGlyph.Bitmap.String())
			ShowSimilarity(t, test.bitmap, closestGlyph.Bitmap)
			similar := calculateSimilarityBatch(extractFeatures(test.bitmap), glyphs)
			for idx := range similar {
				glyph := similar[idx].Target
				t.Logf("Similarity: %f\n%s\n", similar[idx].Similarity, glyph.Bitmap.String())
				ShowSimilarity(t, test.bitmap, glyph.Bitmap)
				if idx > 3 {
					break
				}
			}

			// Add assertions here if you have expected outcomes
		})
	}
}