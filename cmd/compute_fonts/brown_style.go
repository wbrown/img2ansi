package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
)

// TestBrownDitheringStyle tests Brown-style joint optimization for 8x8
func TestBrownDitheringStyle() {
	// Load fonts
	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	// Load and prepare image
	file, err := os.Open("mandrill_original.png")
	if err != nil {
		fmt.Printf("Could not load mandrill image: %v\n", err)
		return
	}
	defer file.Close()
	
	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Printf("Could not decode image: %v\n", err)
		return
	}
	
	// Scale and prepare - target 80x40 characters = 640x320 pixels
	targetSize := image.NewRGBA(image.Rect(0, 0, 640, 640))
	xRatio := float64(img.Bounds().Dx()) / 640.0
	yRatio := float64(img.Bounds().Dy()) / 640.0
	
	for y := 0; y < 640; y++ {
		for x := 0; x < 640; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			targetSize.Set(x, y, img.At(srcX, srcY))
		}
	}
	
	// Resize for terminal aspect ratio
	resized := image.NewRGBA(image.Rect(0, 0, 640, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 640; x++ {
			resized.Set(x, y, targetSize.At(x, y*2))
		}
	}
	
	// Define restricted character set - density-based only
	densityChars := []rune{
		' ',  // 0%
		'░',  // 25%
		'▒',  // 50%
		'▓',  // 75%
		'█',  // 100%
		'▀',  // top half
		'▄',  // bottom half
		'▌',  // left half
		'▐',  // right half
	}
	
	// Process a few blocks with Brown-style optimization
	fmt.Println("=== Brown-style 8x8 optimization ===")
	fmt.Println("\nProcessing center blocks...")
	
	centerY := 20  // Center of 40 rows
	centerX := 40  // Center of 80 cols
	
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			blockY := centerY + dy
			blockX := centerX + dx
			
			fmt.Printf("\nBlock (%d,%d):\n", blockX, blockY)
			
			// Extract 8x8 pixel block
			blockPixels := make([]color.Color, 64)
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					px := blockX*8 + x
					py := blockY*8 + y
					blockPixels[y*8+x] = resized.At(px, py)
				}
			}
			
			// Find best character + color combination
			bestChar := ' '
			bestError := math.MaxFloat64
			bestInverse := false
			
			// Try each character
			for _, char := range densityChars {
				charInfo := lookup.LookupRune(char)
				if charInfo == nil {
					continue
				}
				
				// Extract dominant colors from block
				colors := extractBlockColors(blockPixels)
				if len(colors) >= 2 {
					fg := colors[0]
					bg := colors[1]
					
					// Try normal orientation
					error := calculateBlockError(blockPixels, charInfo.Bitmap, fg, bg)
					if error < bestError {
						bestError = error
						bestChar = char
						bestInverse = false
					}
					
					// Try inverse orientation (swap fg/bg)
					error = calculateBlockError(blockPixels, charInfo.Bitmap, bg, fg)
					if error < bestError {
						bestError = error
						bestChar = char
						bestInverse = true
					}
				}
			}
			
			fmt.Printf("Best: '%c' ", bestChar)
			if bestInverse {
				fmt.Printf("(inverse) ")
			}
			fmt.Printf("error: %.0f\n", bestError)
		}
	}
	
	// Now render the full image
	fmt.Println("\n\nRendering full image with Brown-style optimization...")
	outputFile, _ := os.Create("mandrill_brown_8x8.ans")
	defer outputFile.Close()
	
	height := 40  // 320 pixels / 8
	width := 80   // 640 pixels / 8
	
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Extract 8x8 pixel block
			blockPixels := make([]color.Color, 64)
			for dy := 0; dy < 8; dy++ {
				for dx := 0; dx < 8; dx++ {
					px := x*8 + dx
					py := y*8 + dy
					blockPixels[dy*8+dx] = resized.At(px, py)
				}
			}
			
			// Find best character + color combination
			bestChar := ' '
			var bestFg color.Color = color.Black
			var bestBg color.Color = color.White
			bestError := math.MaxFloat64
			
			// Try each character
			for _, char := range densityChars {
				charInfo := lookup.LookupRune(char)
				if charInfo == nil {
					continue
				}
				
				// Extract dominant colors from block
				colors := extractBlockColors(blockPixels)
				if len(colors) >= 2 {
					fg := colors[0]
					bg := colors[1]
					
					// Try normal orientation
					error := calculateBlockError(blockPixels, charInfo.Bitmap, fg, bg)
					if error < bestError {
						bestError = error
						bestChar = char
						bestFg = fg
						bestBg = bg
					}
					
					// Try inverse orientation (swap fg/bg)
					error = calculateBlockError(blockPixels, charInfo.Bitmap, bg, fg)
					if error < bestError {
						bestError = error
						bestChar = char
						bestFg = bg
						bestBg = fg
					}
				}
			}
			
			// Output ANSI codes
			fgR, fgG, fgB, _ := bestFg.RGBA()
			bgR, bgG, bgB, _ := bestBg.RGBA()
			fgCode := rgbToANSI256(uint8(fgR>>8), uint8(fgG>>8), uint8(fgB>>8))
			bgCode := rgbToANSI256(uint8(bgR>>8), uint8(bgG>>8), uint8(bgB>>8))
			fmt.Fprintf(outputFile, "\x1b[38;5;%dm\x1b[48;5;%dm%c", fgCode, bgCode, bestChar)
		}
		fmt.Fprintln(outputFile, "\x1b[0m")
	}
	
	fmt.Println("\nSaved to mandrill_brown_8x8.ans")
	
	// Also save comparison statistics
	fmt.Println("\nCharacter usage statistics:")
	charCounts := make(map[rune]int)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Extract 8x8 pixel block
			blockPixels := make([]color.Color, 64)
			for dy := 0; dy < 8; dy++ {
				for dx := 0; dx < 8; dx++ {
					px := x*8 + dx
					py := y*8 + dy
					blockPixels[dy*8+dx] = resized.At(px, py)
				}
			}
			
			// Find best character
			bestChar := ' '
			bestError := math.MaxFloat64
			
			for _, char := range densityChars {
				charInfo := lookup.LookupRune(char)
				if charInfo == nil {
					continue
				}
				
				colors := extractBlockColors(blockPixels)
				if len(colors) >= 2 {
					fg := colors[0]
					bg := colors[1]
					
					// Try both orientations
					error1 := calculateBlockError(blockPixels, charInfo.Bitmap, fg, bg)
					error2 := calculateBlockError(blockPixels, charInfo.Bitmap, bg, fg)
					
					if error1 < bestError {
						bestError = error1
						bestChar = char
					}
					if error2 < bestError {
						bestError = error2
						bestChar = char
					}
				}
			}
			
			charCounts[bestChar]++
		}
	}
	
	// Print usage
	total := width * height
	for _, char := range densityChars {
		count := charCounts[char]
		percent := float64(count) / float64(total) * 100
		fmt.Printf("'%c': %d (%.1f%%)\n", char, count, percent)
	}
}

// Functions moved to common.go