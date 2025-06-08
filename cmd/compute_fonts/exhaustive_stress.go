package main

import (
	"fmt"
	"github.com/wbrown/img2ansi"
	"math/rand"
)

// ExhaustiveStressTest creates challenging test cases to show differences
func ExhaustiveStressTest() error {
	fmt.Println("=== Exhaustive Methods Stress Test ===")
	fmt.Println("Testing with challenging color patterns...\n")
	
	// Load palette
	fgTable, _, err := img2ansi.LoadPalette("ansi16")
	if err != nil {
		return err
	}
	
	// Get RGB colors from the palette
	palette := make([]img2ansi.RGB, len(fgTable.AnsiData))
	for i, entry := range fgTable.AnsiData {
		palette[i] = img2ansi.RGB{
			R: uint8((entry.Key >> 16) & 0xFF),
			G: uint8((entry.Key >> 8) & 0xFF),
			B: uint8(entry.Key & 0xFF),
		}
	}
	
	lookup := LoadFontGlyphs()
	chars := getCharacterSet(CharSetDensity, nil)
	
	// Test Case 1: High contrast pattern
	fmt.Println("Test 1: High Contrast Checkerboard Pattern")
	pixels1 := make([]img2ansi.RGB, 64)
	for i := 0; i < 64; i++ {
		x := i % 8
		y := i / 8
		if (x+y)%2 == 0 {
			pixels1[i] = img2ansi.RGB{R: 255, G: 255, B: 255} // White
		} else {
			pixels1[i] = img2ansi.RGB{R: 0, G: 0, B: 0} // Black
		}
	}
	compareExhaustiveMethods(pixels1, palette, chars, lookup, "Checkerboard")
	
	// Test Case 2: Gradient pattern
	fmt.Println("\nTest 2: Gradient Pattern")
	pixels2 := make([]img2ansi.RGB, 64)
	for i := 0; i < 64; i++ {
		x := i % 8
		gray := uint8(x * 255 / 7)
		pixels2[i] = img2ansi.RGB{R: gray, G: gray, B: gray}
	}
	compareExhaustiveMethods(pixels2, palette, chars, lookup, "Gradient")
	
	// Test Case 3: Random noise
	fmt.Println("\nTest 3: Random Noise")
	pixels3 := make([]img2ansi.RGB, 64)
	rand.Seed(42) // Fixed seed for reproducibility
	for i := 0; i < 64; i++ {
		pixels3[i] = img2ansi.RGB{
			R: uint8(rand.Intn(256)),
			G: uint8(rand.Intn(256)),
			B: uint8(rand.Intn(256)),
		}
	}
	compareExhaustiveMethods(pixels3, palette, chars, lookup, "Random")
	
	// Test Case 4: Specific pattern that might benefit from true exhaustive
	fmt.Println("\nTest 4: Mixed Pattern (25% black, 25% white, 50% gray)")
	pixels4 := make([]img2ansi.RGB, 64)
	for i := 0; i < 16; i++ {
		pixels4[i] = img2ansi.RGB{R: 0, G: 0, B: 0} // Black
	}
	for i := 16; i < 32; i++ {
		pixels4[i] = img2ansi.RGB{R: 255, G: 255, B: 255} // White
	}
	for i := 32; i < 64; i++ {
		pixels4[i] = img2ansi.RGB{R: 128, G: 128, B: 128} // Gray
	}
	compareExhaustiveMethods(pixels4, palette, chars, lookup, "Mixed")
	
	return nil
}

func compareExhaustiveMethods(pixels []img2ansi.RGB, palette []img2ansi.RGB, chars []rune, lookup *GlyphLookup, testName string) {
	// Method 1: Current Exhaustive
	selector := &ExhaustiveColorSelector{MaxPairs: 20}
	colorPairs := selector.SelectColors(pixels, palette)
	
	config := BrownDitheringConfig{}
	char1, fg1, bg1 := findBestCharacterMatch(pixels, chars, colorPairs, lookup, config)
	
	glyphInfo1 := lookup.LookupRune(char1)
	error1 := calculateGlyphBlockErrorRGB(pixels, glyphInfo1.Bitmap, fg1, bg1)
	
	// Method 2: True Exhaustive
	var char2 rune
	var fg2, bg2 img2ansi.RGB
	bestError2 := 999999.0
	
	for _, char := range chars {
		glyphInfo := lookup.LookupRune(char)
		if glyphInfo == nil {
			continue
		}
		
		for _, fg := range palette {
			for _, bg := range palette {
				error := calculateGlyphBlockErrorRGB(pixels, glyphInfo.Bitmap, fg, bg)
				if error < bestError2 {
					bestError2 = error
					char2 = char
					fg2 = fg
					bg2 = bg
				}
			}
		}
	}
	
	// Compare results
	fmt.Printf("  Current: '%c' FG(%3d,%3d,%3d) BG(%3d,%3d,%3d) Error=%.1f\n",
		char1, fg1.R, fg1.G, fg1.B, bg1.R, bg1.G, bg1.B, error1)
	fmt.Printf("  True:    '%c' FG(%3d,%3d,%3d) BG(%3d,%3d,%3d) Error=%.1f\n",
		char2, fg2.R, fg2.G, fg2.B, bg2.R, bg2.G, bg2.B, bestError2)
	
	if char1 == char2 && fg1 == fg2 && bg1 == bg2 {
		fmt.Println("  Result: SAME")
	} else {
		improvement := (error1 - bestError2) / error1 * 100
		fmt.Printf("  Result: DIFFERENT - True exhaustive %.1f%% better\n", improvement)
	}
}