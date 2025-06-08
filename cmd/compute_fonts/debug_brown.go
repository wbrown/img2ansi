package main

import (
	"fmt"
	"github.com/wbrown/img2ansi"
)

// DebugBrownDithering runs a single block through different strategies to see differences
func DebugBrownDithering() error {
	// Load image
	resized, err := LoadMandrillImage()
	if err != nil {
		return err
	}
	
	// Load fonts
	lookup := LoadFontGlyphs()
	
	// Get a single 8x8 block from the image (pick an interesting area)
	blockX, blockY := 10, 10 // Adjust to find interesting block
	bounds := resized.Bounds()
	
	// Extract the block
	var pixels []img2ansi.RGB
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			px := blockX*8 + x
			py := blockY*8 + y
			if px < bounds.Max.X && py < bounds.Max.Y {
				c := resized.At(px, py)
				r, g, b, _ := c.RGBA()
				pixels = append(pixels, img2ansi.RGB{
					R: uint8(r >> 8),
					G: uint8(g >> 8),
					B: uint8(b >> 8),
				})
			}
		}
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
	
	fmt.Printf("Testing block at (%d,%d) with %d pixels\n", blockX, blockY, len(pixels))
	fmt.Println("Block colors:")
	for i := 0; i < 8 && i < len(pixels); i++ {
		p := pixels[i]
		fmt.Printf("  Pixel %d: RGB(%d,%d,%d)\n", i, p.R, p.G, p.B)
	}
	
	// Test different color selectors
	selectors := map[string]ColorSelector{
		"Dominant": &DominantColorSelector{},
		"Exhaustive": &ExhaustiveColorSelector{MaxPairs: 20},
		"Quantized": &QuantizedColorSelector{Levels: 4},
		"KMeans": &KMeansColorSelector{K: 4},
		"Contrast": &ContrastColorSelector{MinContrast: 30.0},
	}
	
	// Test with each selector
	for name, selector := range selectors {
		fmt.Printf("\n=== %s Color Selector ===\n", name)
		
		// Get color pairs
		pairs := selector.SelectColors(pixels, palette)
		fmt.Printf("Generated %d color pairs\n", len(pairs))
		
		// Show first few pairs
		for i := 0; i < 3 && i < len(pairs); i++ {
			fmt.Printf("  Pair %d: FG(%3d,%3d,%3d) BG(%3d,%3d,%3d)\n",
				i+1,
				pairs[i].Fg.R, pairs[i].Fg.G, pairs[i].Fg.B,
				pairs[i].Bg.R, pairs[i].Bg.G, pairs[i].Bg.B)
		}
		
		// Test with different character sets
		charSets := map[string][]rune{
			"Density": getCharacterSet(CharSetDensity, nil),
			"NoSpace": getCharacterSet(CharSetNoSpace, nil),
		}
		
		for setName, chars := range charSets {
			// Find best match using this selector's pairs
			config := BrownDitheringConfig{} // dummy config
			bestChar, bestFg, bestBg := findBestCharacterMatch(
				pixels, chars, pairs, lookup, config)
			
			fmt.Printf("  With %s chars: '%c' FG(%d,%d,%d) BG(%d,%d,%d)\n",
				setName, bestChar,
				bestFg.R, bestFg.G, bestFg.B,
				bestBg.R, bestBg.G, bestBg.B)
		}
	}
	
	return nil
}