package main

import (
	"fmt"
	"github.com/wbrown/img2ansi"
	"image"
	"image/color"
	"image/draw"
	"os"
)

// ExhaustiveVisualOutput creates visual .ans files comparing both methods
func ExhaustiveVisualOutput() error {
	fmt.Println("=== Generating Visual Comparison Files ===")
	
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
	
	// Create test patterns
	patterns := []struct {
		name string
		createFunc func() image.Image
	}{
		{
			name: "checkerboard",
			createFunc: func() image.Image {
				img := image.NewRGBA(image.Rect(0, 0, 80, 40))
				for y := 0; y < 40; y++ {
					for x := 0; x < 80; x++ {
						if (x/4+y/4)%2 == 0 {
							img.Set(x, y, color.White)
						} else {
							img.Set(x, y, color.Black)
						}
					}
				}
				return img
			},
		},
		{
			name: "gradient",
			createFunc: func() image.Image {
				img := image.NewRGBA(image.Rect(0, 0, 80, 40))
				for y := 0; y < 40; y++ {
					for x := 0; x < 80; x++ {
						gray := uint8(x * 255 / 79)
						img.Set(x, y, color.RGBA{gray, gray, gray, 255})
					}
				}
				return img
			},
		},
		{
			name: "circles",
			createFunc: func() image.Image {
				img := image.NewRGBA(image.Rect(0, 0, 80, 40))
				// Fill with gray background
				draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{128, 128, 128, 255}}, image.Point{}, draw.Src)
				
				// Draw some circles
				centers := [][2]int{{20, 20}, {60, 20}, {40, 30}}
				for _, c := range centers {
					cx, cy := c[0], c[1]
					for y := 0; y < 40; y++ {
						for x := 0; x < 80; x++ {
							dx := float64(x - cx)
							dy := float64(y - cy)
							if dx*dx+dy*dy <= 100 { // radius 10
								img.Set(x, y, color.White)
							}
						}
					}
				}
				return img
			},
		},
	}
	
	// Process each pattern with both methods
	for _, pattern := range patterns {
		fmt.Printf("\nProcessing %s pattern...\n", pattern.name)
		img := pattern.createFunc()
		
		// Method 1: Current Exhaustive
		fmt.Println("  Running current exhaustive method...")
		blocks1 := processWithCurrentExhaustive(img, palette, chars, lookup)
		ansiOutput1 := img2ansi.RenderToAnsi(blocks1)
		outputFile1 := OutputPath(fmt.Sprintf("%s_current_exhaustive.ans", pattern.name))
		if err := SaveAnsiFile(outputFile1, ansiOutput1); err != nil {
			return err
		}
		fmt.Printf("  Saved to %s\n", outputFile1)
		
		// Method 2: True Exhaustive (limited blocks for speed)
		fmt.Println("  Running true exhaustive method (first 10 blocks)...")
		blocks2 := processWithTrueExhaustive(img, palette, lookup, 10)
		ansiOutput2 := img2ansi.RenderToAnsi(blocks2)
		outputFile2 := OutputPath(fmt.Sprintf("%s_true_exhaustive.ans", pattern.name))
		if err := SaveAnsiFile(outputFile2, ansiOutput2); err != nil {
			return err
		}
		fmt.Printf("  Saved to %s\n", outputFile2)
	}
	
	fmt.Println("\nDone! View the .ans files to see the differences.")
	return nil
}

func processWithCurrentExhaustive(img image.Image, palette []img2ansi.RGB, chars []rune, lookup *GlyphLookup) [][]img2ansi.BlockRune {
	bounds := img.Bounds()
	width := bounds.Dx() / 8
	height := bounds.Dy() / 8
	
	blocks := make([][]img2ansi.BlockRune, height)
	selector := &ExhaustiveColorSelector{MaxPairs: 20}
	config := BrownDitheringConfig{}
	
	for y := 0; y < height; y++ {
		blocks[y] = make([]img2ansi.BlockRune, width)
		for x := 0; x < width; x++ {
			pixels := extractBlockPixelsRGB(img, x, y)
			colorPairs := selector.SelectColors(pixels, palette)
			char, fg, bg := findBestCharacterMatch(pixels, chars, colorPairs, lookup, config)
			
			blocks[y][x] = img2ansi.BlockRune{
				Rune: char,
				FG:   fg,
				BG:   bg,
			}
		}
	}
	
	return blocks
}

func processWithTrueExhaustive(img image.Image, palette []img2ansi.RGB, lookup *GlyphLookup, maxBlocks int) [][]img2ansi.BlockRune {
	bounds := img.Bounds()
	width := bounds.Dx() / 8
	height := bounds.Dy() / 8
	
	blocks := make([][]img2ansi.BlockRune, height)
	chars := getCharacterSet(CharSetDensity, nil)
	processed := 0
	
	for y := 0; y < height; y++ {
		blocks[y] = make([]img2ansi.BlockRune, width)
		for x := 0; x < width; x++ {
			if processed < maxBlocks {
				// True exhaustive for first N blocks
				char, fg, bg, _ := ProcessSingleBlockExhaustive(img, x, y, palette, lookup)
				blocks[y][x] = img2ansi.BlockRune{
					Rune: char,
					FG:   fg,
					BG:   bg,
				}
				processed++
			} else {
				// Use current method for rest (for speed)
				pixels := extractBlockPixelsRGB(img, x, y)
				selector := &ExhaustiveColorSelector{MaxPairs: 20}
				colorPairs := selector.SelectColors(pixels, palette)
				config := BrownDitheringConfig{}
				char, fg, bg := findBestCharacterMatch(pixels, chars, colorPairs, lookup, config)
				
				blocks[y][x] = img2ansi.BlockRune{
					Rune: char,
					FG:   fg,
					BG:   bg,
				}
			}
		}
	}
	
	return blocks
}

// SaveAnsiFile saves ANSI content to a file
func SaveAnsiFile(filename, content string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	_, err = file.WriteString(content)
	return err
}