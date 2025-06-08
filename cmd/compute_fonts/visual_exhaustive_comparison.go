package main

import (
	"fmt"
	"github.com/wbrown/img2ansi"
	"image"
	"os"
)

// VisualExhaustiveComparison creates side-by-side comparison of exhaustive methods
func VisualExhaustiveComparison() error {
	fmt.Println("=== Visual Comparison: Current vs True Exhaustive ===")
	
	// Load image
	resized, err := LoadMandrillImage()
	if err != nil {
		return err
	}
	
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
	
	// Process a single interesting block (10, 10)
	blockX, blockY := 10, 10
	pixels := extractBlockPixelsRGB(resized, blockX, blockY)
	
	fmt.Printf("\nAnalyzing block at (%d, %d)\n", blockX, blockY)
	fmt.Println("Block colors (first 8 pixels):")
	for i := 0; i < 8 && i < len(pixels); i++ {
		p := pixels[i]
		fmt.Printf("  Pixel %d: RGB(%3d,%3d,%3d)\n", i, p.R, p.G, p.B)
	}
	
	// Method 1: Current "Exhaustive" (ExhaustiveColorSelector)
	fmt.Println("\n1. Current Exhaustive Method:")
	selector := &ExhaustiveColorSelector{MaxPairs: 20}
	colorPairs := selector.SelectColors(pixels, palette)
	fmt.Printf("   Generated %d color pairs\n", len(colorPairs))
	
	// Find best match using current method
	config := BrownDitheringConfig{}
	bestChar1, bestFg1, bestBg1 := findBestCharacterMatch(pixels, chars, colorPairs, lookup, config)
	fmt.Printf("   Best match: '%c' with FG(%3d,%3d,%3d) BG(%3d,%3d,%3d)\n",
		bestChar1, bestFg1.R, bestFg1.G, bestFg1.B, bestBg1.R, bestBg1.G, bestBg1.B)
	
	// Calculate error for current method
	glyphInfo1 := lookup.LookupRune(bestChar1)
	error1 := calculateGlyphBlockErrorRGB(pixels, glyphInfo1.Bitmap, bestFg1, bestBg1)
	fmt.Printf("   Total error: %.2f\n", error1)
	
	// Method 2: True Exhaustive
	fmt.Println("\n2. True Exhaustive Method:")
	fmt.Printf("   Testing %d characters × %d × %d colors = %d combinations\n",
		len(chars), len(palette), len(palette), len(chars)*len(palette)*len(palette))
	
	var bestChar2 rune
	var bestFg2, bestBg2 img2ansi.RGB
	bestError2 := 999999.0
	
	tested := 0
	// Actually do true exhaustive search
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
					bestChar2 = char
					bestFg2 = fg
					bestBg2 = bg
				}
				tested++
			}
		}
	}
	
	fmt.Printf("   Tested %d combinations\n", tested)
	fmt.Printf("   Best match: '%c' with FG(%3d,%3d,%3d) BG(%3d,%3d,%3d)\n",
		bestChar2, bestFg2.R, bestFg2.G, bestFg2.B, bestBg2.R, bestBg2.G, bestBg2.B)
	fmt.Printf("   Total error: %.2f\n", bestError2)
	
	// Compare results
	fmt.Println("\n=== Comparison ===")
	if bestChar1 == bestChar2 && bestFg1 == bestFg2 && bestBg1 == bestBg2 {
		fmt.Println("Both methods found the SAME result!")
	} else {
		fmt.Println("Methods found DIFFERENT results!")
		fmt.Printf("Error difference: %.2f (%.1f%% improvement)\n", 
			error1-bestError2, (error1-bestError2)/error1*100)
	}
	
	// Save comparison ANSI
	file, err := os.Create(OutputPath("exhaustive_comparison.ans"))
	if err != nil {
		return err
	}
	defer file.Close()
	
	// Write side-by-side comparison
	fmt.Fprintln(file, "\x1b[0mCurrent Exhaustive:     True Exhaustive:")
	fmt.Fprintf(file, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%c\x1b[0m",
		bestFg1.R, bestFg1.G, bestFg1.B, bestBg1.R, bestBg1.G, bestBg1.B, bestChar1)
	fmt.Fprint(file, "                        ")
	fmt.Fprintf(file, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%c\x1b[0m\n",
		bestFg2.R, bestFg2.G, bestFg2.B, bestBg2.R, bestBg2.G, bestBg2.B, bestChar2)
	
	fmt.Printf("\nComparison saved to %s\n", OutputPath("exhaustive_comparison.ans"))
	
	return nil
}

// ProcessSingleBlockExhaustive processes one block with true exhaustive search
func ProcessSingleBlockExhaustive(img image.Image, blockX, blockY int, palette []img2ansi.RGB, lookup *GlyphLookup) (rune, img2ansi.RGB, img2ansi.RGB, float64) {
	pixels := extractBlockPixelsRGB(img, blockX, blockY)
	chars := getCharacterSet(CharSetAll, nil) // Use all characters
	
	var bestChar rune = ' '
	var bestFg, bestBg img2ansi.RGB
	bestError := 999999.0
	
	// True exhaustive: try every combination
	for _, char := range chars {
		glyphInfo := lookup.LookupRune(char)
		if glyphInfo == nil {
			continue
		}
		
		for _, fg := range palette {
			for _, bg := range palette {
				error := calculateGlyphBlockErrorRGB(pixels, glyphInfo.Bitmap, fg, bg)
				if error < bestError {
					bestError = error
					bestChar = char
					bestFg = fg
					bestBg = bg
				}
			}
		}
	}
	
	return bestChar, bestFg, bestBg, bestError
}