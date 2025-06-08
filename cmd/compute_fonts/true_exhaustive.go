package main

import (
	"fmt"
	"image"
	"math"
	"github.com/wbrown/img2ansi"
	"os"
)

// TrueExhaustiveBrownDithering performs actual exhaustive search
func TrueExhaustiveBrownDithering(img image.Image, lookup *GlyphLookup) error {
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
	
	// Get all available characters (using "All" set for maximum variety)
	chars := getCharacterSet(CharSetAll, nil)
	fmt.Printf("Using %d characters for true exhaustive search\n", len(chars))
	fmt.Printf("Testing %d x %d x %d = %d combinations per block\n",
		len(chars), len(palette), len(palette), len(chars)*len(palette)*len(palette))
	
	// Calculate dimensions
	bounds := img.Bounds()
	width := bounds.Dx() / 8
	height := bounds.Dy() / 8
	
	// Build BlockRune array
	blocks := make([][]img2ansi.BlockRune, height)
	
	// Stats
	charUsage := make(map[rune]int)
	colorPairUsage := make(map[string]int)
	
	// Process first few blocks only (true exhaustive is VERY slow)
	maxBlocks := 10 // Limit for demonstration
	processed := 0
	
	for y := 0; y < height && processed < maxBlocks; y++ {
		blocks[y] = make([]img2ansi.BlockRune, width)
		for x := 0; x < width && processed < maxBlocks; x++ {
			// Extract block pixels
			blockPixels := extractBlockPixelsRGB(img, x, y)
			
			// True exhaustive search
			var bestChar rune
			var bestFg, bestBg img2ansi.RGB
			bestError := math.MaxFloat64
			
			// Try EVERY combination
			for _, char := range chars {
				glyphInfo := lookup.LookupRune(char)
				if glyphInfo == nil {
					continue
				}
				
				// Try every foreground color
				for _, fg := range palette {
					// Try every background color
					for _, bg := range palette {
						error := calculateGlyphBlockErrorRGB(blockPixels, glyphInfo.Bitmap, fg, bg)
						if error < bestError {
							bestError = error
							bestChar = char
							bestFg = fg
							bestBg = bg
						}
					}
				}
			}
			
			// Store result
			blocks[y][x] = img2ansi.BlockRune{
				Rune: bestChar,
				FG:   bestFg,
				BG:   bestBg,
			}
			
			// Update stats
			charUsage[bestChar]++
			colorKey := fmt.Sprintf("FG(%d,%d,%d)-BG(%d,%d,%d)", 
				bestFg.R, bestFg.G, bestFg.B,
				bestBg.R, bestBg.G, bestBg.B)
			colorPairUsage[colorKey]++
			
			processed++
			fmt.Printf("Block %d: '%c' with %s\n", processed, bestChar, colorKey)
		}
	}
	
	// Save partial result
	outputFile := OutputPath("brown_true_exhaustive.ans")
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()
	
	ansiOutput := img2ansi.RenderToAnsi(blocks[:processed/width+1])
	file.WriteString(ansiOutput)
	
	fmt.Printf("\nTrue exhaustive search (partial) saved to %s\n", outputFile)
	fmt.Printf("Character usage:\n")
	for char, count := range charUsage {
		fmt.Printf("  '%c': %d times\n", char, count)
	}
	
	return nil
}

// Add a new command to run true exhaustive
func init() {
	// This will be called from main() with "true-exhaustive" command
}