package main

import (
	"github.com/golang/freetype/truetype"
	"testing"
)

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

func ShowSimilarity(t *testing.T, a GlyphBitmap, b GlyphBitmap) {
	aFeatures := extractFeatures(a)
	bFeatures := extractFeatures(b)

	t.Logf("Similarity: %f\n", calculateSimilarity(aFeatures, bFeatures))
	t.Logf("Shape Similarity: %f\n", calculateShapeSimilarity(aFeatures.Bitmap, bFeatures.Bitmap))
	t.Logf("Pattern Similarity: %f\n", calculatePatternSimilarity(aFeatures.Bitmap, bFeatures.Bitmap))
	t.Logf("Density Similarity: %f\n", calculateDensitySimilarity(aFeatures.Bitmap, bFeatures.Bitmap))

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
