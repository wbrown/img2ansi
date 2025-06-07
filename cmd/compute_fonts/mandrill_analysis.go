package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	_ "image/jpeg" // Register JPEG decoder
	"math"
	"math/bits"
	"os"
	"sort"
	
	"github.com/golang/freetype/truetype"
)

// AtkinsonDither applies Atkinson dithering to convert to 1-bit (black/white)
func AtkinsonDither(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	
	// First convert to grayscale
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			gray.Set(x, y, c)
		}
	}
	
	// Apply Atkinson dithering
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			oldPixel := float64(gray.GrayAt(x, y).Y)
			newPixel := float64(0)
			if oldPixel > 127 {
				newPixel = 255
			}
			
			gray.SetGray(x, y, color.Gray{uint8(newPixel)})
			
			// Atkinson distributes error to 6 neighbors (only 75% of error)
			error := (oldPixel - newPixel) / 8.0
			
			// Distribute error (1/8 to each of 6 pixels)
			distributeError(gray, x+1, y, error)
			distributeError(gray, x+2, y, error)
			distributeError(gray, x-1, y+1, error)
			distributeError(gray, x, y+1, error)
			distributeError(gray, x+1, y+1, error)
			distributeError(gray, x, y+2, error)
		}
	}
	
	return gray
}

func distributeError(img *image.Gray, x, y int, error float64) {
	bounds := img.Bounds()
	if x >= bounds.Min.X && x < bounds.Max.X && y >= bounds.Min.Y && y < bounds.Max.Y {
		current := float64(img.GrayAt(x, y).Y)
		new := current + error
		if new < 0 {
			new = 0
		} else if new > 255 {
			new = 255
		}
		img.SetGray(x, y, color.Gray{uint8(new)})
	}
}

// Segment8x8 divides an image into 8x8 blocks
func Segment8x8(img image.Image) [][]GlyphBitmap {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	
	// Calculate grid dimensions
	gridWidth := (width + 7) / 8
	gridHeight := (height + 7) / 8
	
	blocks := make([][]GlyphBitmap, gridHeight)
	for row := range blocks {
		blocks[row] = make([]GlyphBitmap, gridWidth)
	}
	
	// Extract each 8x8 block
	for by := 0; by < gridHeight; by++ {
		for bx := 0; bx < gridWidth; bx++ {
			bitmap := GlyphBitmap(0)
			
			// Extract pixels for this block
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					px := bx*8 + x
					py := by*8 + y
					
					if px < width && py < height {
						c := color.GrayModel.Convert(img.At(px, py)).(color.Gray)
						if c.Y > 127 { // White pixel
							bitmap |= 1 << (y*8 + x)
						}
					}
				}
			}
			
			blocks[by][bx] = bitmap
		}
	}
	
	return blocks
}

// ResizeForTerminal resizes image accounting for 2:1 character aspect ratio
func ResizeForTerminal(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	
	// Scale height by 0.5 to account for 2:1 character aspect ratio
	newHeight := height / 2
	
	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, width, newHeight))
	
	// Simple nearest-neighbor scaling
	for y := 0; y < newHeight; y++ {
		for x := 0; x < width; x++ {
			// Sample from the original image
			srcY := y * 2
			dst.Set(x, y, img.At(x, srcY))
		}
	}
	
	return dst
}

// ColorBlock represents an 8x8 block with its dominant colors
type ColorBlock struct {
	Foreground color.Color
	Background color.Color
	Pattern    GlyphBitmap
}

// ExtractDominantColors finds the two most common colors in a block
func ExtractDominantColors(img image.Image, x, y int) (fg, bg color.Color) {
	// Count color occurrences
	colorCounts := make(map[color.Color]int)
	
	for dy := 0; dy < 8; dy++ {
		for dx := 0; dx < 8; dx++ {
			px := x*8 + dx
			py := y*8 + dy
			
			if px < img.Bounds().Dx() && py < img.Bounds().Dy() {
				c := img.At(px, py)
				// Quantize to reduce color space
				r, g, b, _ := c.RGBA()
				// Simple quantization to 6 bits per channel
				qr := (r >> 10) << 10
				qg := (g >> 10) << 10
				qb := (b >> 10) << 10
				qc := color.RGBA{uint8(qr >> 8), uint8(qg >> 8), uint8(qb >> 8), 255}
				colorCounts[qc]++
			}
		}
	}
	
	// Find two most common colors
	type colorCount struct {
		color color.Color
		count int
	}
	
	var sorted []colorCount
	for c, count := range colorCounts {
		sorted = append(sorted, colorCount{c, count})
	}
	
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})
	
	if len(sorted) >= 2 {
		fg = sorted[0].color
		bg = sorted[1].color
	} else if len(sorted) == 1 {
		fg = sorted[0].color
		bg = color.Black
	} else {
		fg = color.White
		bg = color.Black
	}
	
	return fg, bg
}

// CreatePatternFromColors creates a bitmap pattern based on which pixels are closer to fg vs bg
func CreatePatternFromColors(img image.Image, x, y int, fg, bg color.Color) GlyphBitmap {
	var pattern GlyphBitmap
	
	for dy := 0; dy < 8; dy++ {
		for dx := 0; dx < 8; dx++ {
			px := x*8 + dx
			py := y*8 + dy
			
			if px < img.Bounds().Dx() && py < img.Bounds().Dy() {
				c := img.At(px, py)
				
				// Calculate distance to fg and bg
				fgDist := colorDistance(c, fg)
				bgDist := colorDistance(c, bg)
				
				// Set bit if closer to foreground
				if fgDist < bgDist {
					pattern |= 1 << (dy*8 + dx)
				}
			}
		}
	}
	
	return pattern
}

// colorDistance moved to common.go

// colorToANSI256 converts color.Color to ANSI - wrapper for common function
func colorToANSI256(c color.Color) int {
	r, g, b, _ := c.RGBA()
	return rgbToANSI256(uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// AnalyzeSegments runs glyph matching on each segment and analyzes results
func AnalyzeSegments(blocks [][]GlyphBitmap, lookup *GlyphLookup) {
	// Statistics
	matchCounts := make(map[rune]int)
	densityBuckets := make([]int, 11) // 0-10%, 10-20%, ..., 90-100%
	
	totalBlocks := 0
	for _, row := range blocks {
		for _, block := range row {
			totalBlocks++
			
			// Find best match
			match := lookup.FindClosestGlyph(block)
			matchCounts[match.Rune]++
			
			// Track density
			density := float64(bits.OnesCount64(uint64(block))) / 64.0
			bucket := int(density * 10)
			if bucket > 10 {
				bucket = 10
			}
			densityBuckets[bucket]++
		}
	}
	
	fmt.Printf("\nMandrill Segmentation Analysis\n")
	fmt.Printf("==============================\n")
	fmt.Printf("Image divided into %d blocks\n", totalBlocks)
	
	// Show density distribution
	fmt.Printf("\nDensity distribution:\n")
	for i, count := range densityBuckets {
		if count > 0 {
			fmt.Printf("[%d%%-%d%%]: %d blocks\n", i*10, (i+1)*10, count)
		}
	}
	
	// Show top matched characters
	fmt.Printf("\nTop 20 matched characters:\n")
	type runeCount struct {
		r rune
		count int
	}
	
	var sorted []runeCount
	for r, c := range matchCounts {
		sorted = append(sorted, runeCount{r, c})
	}
	
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})
	
	for i := 0; i < 20 && i < len(sorted); i++ {
		rc := sorted[i]
		percent := float64(rc.count) / float64(totalBlocks) * 100
		fmt.Printf("%2d. '%c' (U+%04X): %d times (%.1f%%)\n", 
			i+1, rc.r, rc.r, rc.count, percent)
	}
}

// AnalyzeSegmentsRestricted runs glyph matching with a restricted character set
func AnalyzeSegmentsRestricted(blocks [][]GlyphBitmap, lookup *GlyphLookup, allowedChars map[rune]bool) {
	// Statistics
	matchCounts := make(map[rune]int)
	densityBuckets := make([]int, 11) // 0-10%, 10-20%, ..., 90-100%
	
	totalBlocks := 0
	for _, row := range blocks {
		for _, block := range row {
			totalBlocks++
			
			// Find best match from allowed characters only
			match := lookup.FindClosestGlyphRestricted(block, allowedChars)
			matchCounts[match.Rune]++
			
			// Track density
			density := float64(bits.OnesCount64(uint64(block))) / 64.0
			bucket := int(density * 10)
			if bucket > 10 {
				bucket = 10
			}
			densityBuckets[bucket]++
		}
	}
	
	fmt.Printf("\nMandrill Segmentation Analysis (Restricted Character Set)\n")
	fmt.Printf("=========================================================\n")
	fmt.Printf("Image divided into %d blocks\n", totalBlocks)
	fmt.Printf("Using %d allowed characters\n", len(allowedChars))
	
	// Show density distribution
	fmt.Printf("\nDensity distribution:\n")
	for i, count := range densityBuckets {
		if count > 0 {
			fmt.Printf("[%d%%-%d%%]: %d blocks\n", i*10, (i+1)*10, count)
		}
	}
	
	// Show top matched characters
	fmt.Printf("\nTop matched characters:\n")
	type runeCount struct {
		r rune
		count int
	}
	
	var sorted []runeCount
	for r, c := range matchCounts {
		sorted = append(sorted, runeCount{r, c})
	}
	
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})
	
	for i := 0; i < len(sorted); i++ {
		rc := sorted[i]
		percent := float64(rc.count) / float64(totalBlocks) * 100
		fmt.Printf("%2d. '%c' (U+%04X): %d times (%.1f%%)\n", 
			i+1, rc.r, rc.r, rc.count, percent)
	}
}

// VisualizeMatches creates an output showing the matched characters
func VisualizeMatches(blocks [][]GlyphBitmap, lookup *GlyphLookup) {
	fmt.Printf("\nFirst 10x10 block grid:\n")
	fmt.Printf("======================\n")
	
	maxRows := 10
	maxCols := 10
	
	if len(blocks) < maxRows {
		maxRows = len(blocks)
	}
	
	for row := 0; row < maxRows; row++ {
		if len(blocks[row]) < maxCols {
			maxCols = len(blocks[row])
		}
		
		for col := 0; col < maxCols; col++ {
			match := lookup.FindClosestGlyph(blocks[row][col])
			fmt.Printf("%c", match.Rune)
		}
		fmt.Println()
	}
}

// VisualizeMatchesRestricted creates output with restricted character set
func VisualizeMatchesRestricted(blocks [][]GlyphBitmap, lookup *GlyphLookup, allowedChars map[rune]bool) {
	fmt.Printf("\nFirst 10x10 block grid (Restricted):\n")
	fmt.Printf("====================================\n")
	
	maxRows := 10
	maxCols := 10
	
	if len(blocks) < maxRows {
		maxRows = len(blocks)
	}
	
	for row := 0; row < maxRows; row++ {
		if len(blocks[row]) < maxCols {
			maxCols = len(blocks[row])
		}
		
		for col := 0; col < maxCols; col++ {
			match := lookup.FindClosestGlyphRestricted(blocks[row][col], allowedChars)
			fmt.Printf("%c", match.Rune)
		}
		fmt.Println()
	}
}

// SaveFullOutput saves the complete output to a file
func SaveFullOutput(blocks [][]GlyphBitmap, lookup *GlyphLookup, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	for _, blockRow := range blocks {
		for _, block := range blockRow {
			match := lookup.FindClosestGlyph(block)
			fmt.Fprintf(file, "%c", match.Rune)
		}
		fmt.Fprintln(file)
	}
	
	return nil
}

// SaveFullOutputRestricted saves the complete output with restricted chars
func SaveFullOutputRestricted(blocks [][]GlyphBitmap, lookup *GlyphLookup, allowedChars map[rune]bool, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	for _, blockRow := range blocks {
		for _, block := range blockRow {
			match := lookup.FindClosestGlyphRestricted(block, allowedChars)
			fmt.Fprintf(file, "%c", match.Rune)
		}
		fmt.Fprintln(file)
	}
	
	return nil
}

// CompareDitheringMethods compares different dithering approaches
func CompareDitheringMethods(img image.Image, lookup *GlyphLookup) {
	// 1. Direct threshold
	threshold := applyThreshold(img, 127)
	blocks1 := Segment8x8(threshold)
	
	fmt.Println("\n1. Simple Threshold (127):")
	AnalyzeSegments(blocks1, lookup)
	
	// 2. Atkinson dithering
	atkinson := AtkinsonDither(img)
	blocks2 := Segment8x8(atkinson)
	
	fmt.Println("\n2. Atkinson Dithering:")
	AnalyzeSegments(blocks2, lookup)
	
	// Show visual comparison
	fmt.Println("\nVisual Comparison (first 10x10):")
	fmt.Println("\nThreshold:")
	VisualizeMatches(blocks1, lookup)
	
	fmt.Println("\nAtkinson:")
	VisualizeMatches(blocks2, lookup)
}

func applyThreshold(img image.Image, threshold uint8) image.Image {
	bounds := img.Bounds()
	out := image.NewGray(bounds)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			if c.Y > threshold {
				out.Set(x, y, color.White)
			} else {
				out.Set(x, y, color.Black)
			}
		}
	}
	
	return out
}

// SaveDitheredImage saves the dithered result for inspection
func SaveDitheredImage(img image.Image, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	
	return png.Encode(f, img)
}

// AnalyzeSpecificBlocks picks specific 8x8 blocks and shows their matches
// // TestBrownDitheringStyleMA tests Brown-style joint optimization for 8x8
// func TestBrownDitheringStyleMA() {
// 	// Load fonts
// 	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
// 	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
// 	glyphs := analyzeFont(font, safeFont)
// 	lookup := NewGlyphLookup(glyphs)
// 	
// 	// Load and prepare image
// 	file, err := os.Open("mandrill_original.png")
// 	if err != nil {
// 		fmt.Printf("Could not load mandrill image: %v\n", err)
// 		return
// 	}
// 	defer file.Close()
// 	
// 	img, _, err := image.Decode(file)
// 	if err != nil {
// 		fmt.Printf("Could not decode image: %v\n", err)
// 		return
// 	}
// 	
// 	// Scale and prepare
// 	targetSize := image.NewRGBA(image.Rect(0, 0, 640, 640))
// 	xRatio := float64(img.Bounds().Dx()) / 640.0
// 	yRatio := float64(img.Bounds().Dy()) / 640.0
// 	
// 	for y := 0; y < 640; y++ {
// 		for x := 0; x < 640; x++ {
// 			srcX := int(float64(x) * xRatio)
// 			srcY := int(float64(y) * yRatio)
// 			targetSize.Set(x, y, img.At(srcX, srcY))
// 		}
// 	}
// 	
// 	resized := ResizeForTerminal(targetSize)
// 	
// 	// Define restricted character set - density-based only
// 	densityChars := []rune{
// 		' ',  // 0%
// 		'░',  // 25%
// 		'▒',  // 50%
// 		'▓',  // 75%
// 		'█',  // 100%
// 		'▀',  // top half
// 		'▄',  // bottom half
// 		'▌',  // left half
// 		'▐',  // right half
// 	}
// 	
// 	// Process a few blocks with Brown-style optimization
// 	fmt.Println("=== Brown-style 8x8 optimization ===")
// 	fmt.Println("\nProcessing center blocks...")
// 	
// 	centerY := resized.Bounds().Dy() / 16  // 8x8 blocks
// 	centerX := resized.Bounds().Dx() / 16
// 	
// 	for dy := -1; dy <= 1; dy++ {
// 		for dx := -1; dx <= 1; dx++ {
// 			blockY := centerY + dy
// 			blockX := centerX + dx
// 			
// 			fmt.Printf("\nBlock (%d,%d):\n", blockX, blockY)
// 			
// 			// Extract 8x8 pixel block
// 			blockPixels := make([]color.Color, 64)
// 			for y := 0; y < 8; y++ {
// 				for x := 0; x < 8; x++ {
// 					px := blockX*8 + x
// 					py := blockY*8 + y
// 					blockPixels[y*8+x] = resized.At(px, py)
// 				}
// 			}
// 			
// 			// Find best character + color combination
// 			bestChar := ' '
// 			bestFg := color.Black
// 			bestBg := color.Black
// 			bestError := math.MaxFloat64
// 			bestInverse := false
// 			
// 			// Try each character
// 			for _, char := range densityChars {
// 				charInfo := lookup.LookupRune(char)
// 				if charInfo == nil {
// 					continue
// 				}
// 				
// 				// Extract dominant colors from block
// 				fg, bg := ExtractDominantColors(resized, blockX, blockY)
// 				
// 				// Try normal orientation
// 				error := calculateBlockErrorMA(blockPixels, charInfo.Bitmap, fg, bg)
// 				if error < bestError {
// 					bestError = error
// 					bestChar = char
// 					bestFg = fg
// 					bestBg = bg
// 					bestInverse = false
// 				}
// 				
// 				// Try inverse orientation (swap fg/bg)
// 				error = calculateBlockError(blockPixels, charInfo.Bitmap, bg, fg)
// 				if error < bestError {
// 					bestError = error
// 					bestChar = char
// 					bestFg = bg
// 					bestBg = fg
// 					bestInverse = true
// 				}
// 			}
// 			
// 			fmt.Printf("Best: '%c' ", bestChar)
// 			if bestInverse {
// 				fmt.Printf("(inverse) ")
// 			}
// 			fmt.Printf("error: %.0f\n", bestError)
// 		}
// 	}
// 	
// 	// Now render the full image
// 	fmt.Println("\n\nRendering full image with Brown-style optimization...")
// 	outputFile, _ := os.Create("mandrill_brown_8x8.ans")
// 	defer outputFile.Close()
// 	
// 	height := resized.Bounds().Dy() / 8
// 	width := resized.Bounds().Dx() / 8
// 	
// 	for y := 0; y < height; y++ {
// 		for x := 0; x < width; x++ {
// 			// Extract 8x8 pixel block
// 			blockPixels := make([]color.Color, 64)
// 			for dy := 0; dy < 8; dy++ {
// 				for dx := 0; dx < 8; dx++ {
// 					px := x*8 + dx
// 					py := y*8 + dy
// 					blockPixels[dy*8+dx] = resized.At(px, py)
// 				}
// 			}
// 			
// 			// Find best character + color combination
// 			bestChar := ' '
// 			bestFg := color.Black
// 			bestBg := color.Black
// 			bestError := math.MaxFloat64
// 			
// 			// Try each character
// 			for _, char := range densityChars {
// 				charInfo := lookup.LookupRune(char)
// 				if charInfo == nil {
// 					continue
// 				}
// 				
// 				// Extract dominant colors from block
// 				fg, bg := ExtractDominantColors(resized, x, y)
// 				
// 				// Try normal orientation
// 				error := calculateBlockErrorMA(blockPixels, charInfo.Bitmap, fg, bg)
// 				if error < bestError {
// 					bestError = error
// 					bestChar = char
// 					bestFg = fg
// 					bestBg = bg
// 				}
// 				
// 				// Try inverse orientation (swap fg/bg)
// 				error = calculateBlockError(blockPixels, charInfo.Bitmap, bg, fg)
// 				if error < bestError {
// 					bestError = error
// 					bestChar = char
// 					bestFg = bg
// 					bestBg = fg
// 				}
// 			}
// 			
// 			// Output ANSI codes
// 			fgCode := colorToANSI256(bestFg)
// 			bgCode := colorToANSI256(bestBg)
// 			fmt.Fprintf(outputFile, "\x1b[38;5;%dm\x1b[48;5;%dm%c", fgCode, bgCode, bestChar)
// 		}
// 		fmt.Fprintln(outputFile, "\x1b[0m")
// 	}
// 	
// 	fmt.Println("\nSaved to mandrill_brown_8x8.ans")
// }

// // calculateBlockErrorMA calculates total pixel error for a character
// func calculateBlockErrorMA(pixels []color.Color, charBitmap GlyphBitmap, fg, bg color.Color) float64 {
// 	var totalError float64
// 	
// 	for i := 0; i < 64; i++ {
// 		y := i / 8
// 		x := i % 8
// 		
// 		// What color does the character show at this position?
// 		var charColor color.Color
// 		if getBit(charBitmap, x, y) {
// 			charColor = fg
// 		} else {
// 			charColor = bg
// 		}
// 		
// 		// Calculate error vs actual pixel
// 		totalError += colorDistanceMA(pixels[i], charColor)
// 	}
// 	
// 	return totalError
// }

// AnalyzeOneProblem shows detailed analysis of one problematic match
func AnalyzeOneProblem() {
	// Load fonts
	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	
	// Debug the ▁ character specifically
	fmt.Println("=== Debugging ▁ (U+2581) Rendering ===")
	
	// Render just this one character
	debugRune := '▁'
	fmt.Printf("\nRendering '%c' (U+%04X)...\n", debugRune, debugRune)
	
	// Try both fonts
	bitmap1 := renderGlyphSimple(font, debugRune)
	fmt.Printf("\nIBM BIOS font bitmap: %064b\n", uint64(bitmap1))
	fmt.Println("Visual:")
	fmt.Println(bitmap1.String())
	
	bitmap2 := renderGlyphSimple(safeFont, debugRune) 
	fmt.Printf("\nPragmataPro font bitmap: %064b\n", uint64(bitmap2))
	fmt.Println("Visual:")
	fmt.Println(bitmap2.String())
	
	// Now let's check other line characters
	lineChars := []rune{'_', '‾', '¯', '-'}
	fmt.Println("\n=== Checking line characters ===")
	for _, r := range lineChars {
		bitmap := renderGlyphSimple(font, r)
		fmt.Printf("\n'%c' (U+%04X):\n", r, r)
		fmt.Println(bitmap.String())
	}
	
	// Now let's create the inverted pattern from block 39,21
	fmt.Println("\n=== The pattern we're matching against ===")
	// Manually create the inverted pattern from block 39,21
	var invertedPattern GlyphBitmap
	patternStr := []string{
		"██··██··",
		"·····███",
		"·███····",
		"█···█·█·",
		"··█···█·",
		"··█··█·█",
		"██··██·█",
		"█··█··█·",
	}
	
	for y, row := range patternStr {
		for x, ch := range row {
			if ch == '█' {
				invertedPattern |= 1 << (y*8 + x)
			}
		}
	}
	
	fmt.Println("Inverted pattern from block 39,21:")
	fmt.Println(invertedPattern.String())
	
	// Calculate similarities
	fmt.Println("\n=== Similarity calculations ===")
	for _, r := range append(lineChars, '▁') {
		bitmap := renderGlyphSimple(font, r)
		matching := ^(invertedPattern ^ bitmap)
		matchCount := bits.OnesCount64(uint64(matching))
		similarity := float64(matchCount) / 64.0
		fmt.Printf("'%c': %.4f (%d/64 matching bits)\n", r, similarity, matchCount)
	}
	
	return // Skip the rest for now
	
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
	
	// Scale to target size
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
	
	// Resize for terminal
	resized := ResizeForTerminal(targetSize)
	
	// Apply Atkinson dithering
	atkinson := AtkinsonDither(resized)
	blocks := Segment8x8(atkinson)
	
	// Get block 39,21
	block := blocks[21][39]
	inverted := ^block
	
	fmt.Println("=== Detailed Analysis of Block 39,21 Inverted ===")
	fmt.Println("\nOriginal block (39,21):")
	fmt.Println(block.String())
	
	fmt.Println("\nInverted block:")
	fmt.Println(inverted.String())
	
	// Show bit-by-bit for inverted
	fmt.Println("\nInverted as 64-bit binary:")
	fmt.Printf("%064b\n", uint64(inverted))
	
	// Get ▁ character
	lowerBlock := lookup.LookupRune('▁')
	if lowerBlock != nil {
		fmt.Println("\n'▁' (U+2581) Lower one eighth block:")
		fmt.Println(lowerBlock.Bitmap.String())
		
		fmt.Println("\n'▁' as 64-bit binary:")
		fmt.Printf("%064b\n", uint64(lowerBlock.Bitmap))
		
		// Calculate similarity manually
		matching := ^(inverted ^ lowerBlock.Bitmap)
		matchCount := bits.OnesCount64(uint64(matching))
		similarity := float64(matchCount) / 64.0
		
		fmt.Printf("\nManual calculation:\n")
		fmt.Printf("Inverted:  %064b\n", uint64(inverted))
		fmt.Printf("▁:         %064b\n", uint64(lowerBlock.Bitmap))
		fmt.Printf("XOR:       %064b\n", uint64(inverted ^ lowerBlock.Bitmap))
		fmt.Printf("Matching:  %064b\n", uint64(matching))
		fmt.Printf("Match count: %d / 64\n", matchCount)
		fmt.Printf("Similarity: %.4f\n", similarity)
	}
}

func AnalyzeSpecificBlocks() {
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
	
	// Scale to target size
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
	
	// Resize for terminal
	resized := ResizeForTerminal(targetSize)
	
	// Apply Atkinson dithering
	atkinson := AtkinsonDither(resized)
	blocks := Segment8x8(atkinson)
	
	// Pick 10 blocks from the eye/nose area (high density)
	// This is roughly the center of the image
	centerY := len(blocks) / 2
	centerX := len(blocks[0]) / 2
	
	fmt.Println("\n=== Analyzing 10 specific 8x8 blocks from high-density area ===")
	fmt.Println("\nEach block shows:")
	fmt.Println("1. The 8x8 bitmap after Atkinson dithering")
	fmt.Println("2. Top 4 character matches with similarity scores")
	fmt.Println("")
	
	allowedChars := CreateRestrictedCharSet()
	
	// Analyze specific blocks including 39,39
	testBlocks := []struct{x, y int}{
		{39, 39}, // The suspicious one
		{centerX-1, centerY-2},
		{centerX, centerY-2},
		{centerX-1, centerY-1},
		{centerX, centerY-1},
		{centerX-1, centerY},
		{centerX, centerY},
		{centerX-1, centerY+1},
		{centerX, centerY+1},
		{centerX-1, centerY+2},
	}
	
	for blockNum, pos := range testBlocks {
		x, y := pos.x, pos.y
			
			if y >= 0 && y < len(blocks) && x >= 0 && x < len(blocks[0]) {
				block := blocks[y][x]
				
				fmt.Printf("\n--- Block %d (position: %d,%d) ---\n", blockNum+1, x, y)
				fmt.Println("Original bitmap:")
				fmt.Println(block.String())
				
				// Get top 4 matches for original
				matches := lookup.FindTopNGlyphsRestricted(block, allowedChars, 4)
				
				fmt.Println("\nTop 4 matches (NORMAL):")
				for i, match := range matches {
					glyph := lookup.LookupRune(match.Rune)
					if glyph != nil {
						fmt.Printf("%d. '%c' (U+%04X) - similarity: %.4f\n", 
							i+1, match.Rune, match.Rune, match.Similarity)
					}
				}
				
				// Test inverted bitmap
				inverted := ^block
				fmt.Println("\nInverted bitmap:")
				fmt.Println(inverted.String())
				
				// Get top 4 matches for inverted
				invertedMatches := lookup.FindTopNGlyphsRestricted(inverted, allowedChars, 4)
				
				fmt.Println("\nTop 4 matches (INVERTED):")
				for i, match := range invertedMatches {
					glyph := lookup.LookupRune(match.Rune)
					if glyph != nil {
						fmt.Printf("%d. '%c' (U+%04X) - similarity: %.4f\n", 
							i+1, match.Rune, match.Rune, match.Similarity)
					}
				}
				
				// Compare best scores
				bestNormal := matches[0].Similarity
				bestInverted := invertedMatches[0].Similarity
				
				if bestInverted > bestNormal {
					fmt.Printf("\n>>> INVERTED is better! (%.4f vs %.4f)\n", bestInverted, bestNormal)
					fmt.Printf(">>> Best choice: '%c' with fg/bg swapped\n", invertedMatches[0].Rune)
				} else {
					fmt.Printf("\n>>> NORMAL is better (%.4f vs %.4f)\n", bestNormal, bestInverted)
					fmt.Printf(">>> Best choice: '%c' as-is\n", matches[0].Rune)
				}
			}
		}
}

// CreateRestrictedCharSet creates a set of allowed characters without semantic distractions
func CreateRestrictedCharSet() map[rune]bool {
	allowed := make(map[rune]bool)
	
	// Core density gradient characters (0%, 25%, 50%, 75%, 100%)
	allowed[' '] = true  // Space (0% density)
	allowed['░'] = true  // Light shade (25% density)
	allowed['▒'] = true  // Medium shade (50% density)
	allowed['▓'] = true  // Dark shade (75% density)
	allowed['█'] = true  // Full block (100% density)
	
	// Half blocks (available in CP437)
	allowed['▀'] = true  // Upper half block
	allowed['▄'] = true  // Lower half block
	allowed['▌'] = true  // Left half block
	allowed['▐'] = true  // Right half block
	allowed['▁'] = true  // Lower one eighth block
	
	// Geometric shapes
	allowed['■'] = true  // Black square
	allowed['□'] = true  // White square
	allowed['▪'] = true  // Black small square
	allowed['▫'] = true  // White small square
	allowed['●'] = true  // Black circle
	allowed['○'] = true  // White circle
	allowed['◦'] = true  // White bullet
	allowed['•'] = true  // Bullet
	allowed['◘'] = true  // Inverse bullet
	allowed['◙'] = true  // Inverse white circle
	allowed['▲'] = true  // Black up-pointing triangle
	allowed['▼'] = true  // Black down-pointing triangle
	allowed['►'] = true  // Black right-pointing pointer
	allowed['◄'] = true  // Black left-pointing pointer
	allowed['◆'] = true  // Black diamond
	allowed['◊'] = true  // Lozenge
	
	// Basic ASCII punctuation and symbols (for pattern variety)
	allowed['.'] = true  // Period (single dot)
	allowed[':'] = true  // Colon (two dots)
	allowed['='] = true  // Equals (horizontal lines)
	allowed['+'] = true  // Plus (cross pattern)
	allowed['*'] = true  // Asterisk (radial pattern)
	allowed['#'] = true  // Hash (grid pattern)
	allowed['@'] = true  // At (circular pattern)
	allowed['%'] = true  // Percent (diagonal pattern)
	allowed['&'] = true  // Ampersand (complex pattern)
	allowed['$'] = true  // Dollar (vertical pattern)
	allowed['-'] = true  // Dash (horizontal line)
	allowed['_'] = true  // Underscore (bottom line)
	allowed['|'] = true  // Pipe (vertical line)
	allowed['/'] = true  // Forward slash
	allowed['\\'] = true // Backslash
	allowed['^'] = true  // Caret
	allowed['~'] = true  // Tilde
	allowed['°'] = true  // Degree sign (small circle)
	allowed['¦'] = true  // Broken bar
	allowed['≡'] = true  // Triple horizontal lines
	allowed['∩'] = true  // Intersection
	allowed['¬'] = true  // Not sign
	allowed['⌐'] = true  // Reversed not sign
	allowed['∟'] = true  // Right angle
	
	// Box drawing characters - single lines
	boxSingle := []rune{
		'─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼',
	}
	for _, r := range boxSingle {
		allowed[r] = true
	}
	
	// Box drawing characters - double lines
	boxDouble := []rune{
		'═', '║', '╔', '╗', '╚', '╝', '╠', '╣', '╦', '╩', '╬',
	}
	for _, r := range boxDouble {
		allowed[r] = true
	}
	
	// Box drawing characters - mixed single/double
	boxMixed := []rune{
		'╒', '╓', '╕', '╖', '╘', '╙', '╛', '╜',
		'╞', '╟', '╡', '╢', '╤', '╥', '╧', '╨', '╪', '╫',
	}
	for _, r := range boxMixed {
		allowed[r] = true
	}
	
	// Line and bar characters
	lineChars := []rune{
		'―', '—', '‾', '¯', '‗', '▬',
	}
	for _, r := range lineChars {
		allowed[r] = true
	}
	
	// Special pattern characters
	allowed['…'] = true  // Horizontal ellipsis
	allowed['‹'] = true  // Single left angle quotation
	allowed['›'] = true  // Single right angle quotation
	
	return allowed
}

// Add this to test or main
func TestMandrillMatching() {
	// Load fonts
	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	// Load the baboon image
	fmt.Println("Loading baboon test image...")
	
	file, err := os.Open("../../examples/baboon_256.png")
	if err != nil {
		fmt.Printf("Could not load baboon image: %v\n", err)
		fmt.Println("Using synthetic test pattern instead")
		testImg := createTestPattern()
		CompareDitheringMethods(testImg, lookup)
		return
	}
	defer file.Close()
	
	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Printf("Could not decode image: %v\n", err)
		return
	}
	
	// Save dithered versions
	fmt.Println("Applying Atkinson dithering...")
	atkinson := AtkinsonDither(img)
	SaveDitheredImage(atkinson, "baboon_atkinson.png")
	
	threshold := applyThreshold(img, 127)
	SaveDitheredImage(threshold, "baboon_threshold.png")
	
	// Analyze
	CompareDitheringMethods(img, lookup)
}

// TestMandrillRestricted tests with restricted character set
func TestMandrillRestricted() {
	// Load fonts
	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	// Create restricted character set
	allowedChars := CreateRestrictedCharSet()
	fmt.Printf("\nUsing restricted character set with %d characters\n", len(allowedChars))
	
	// Load the mandrill image
	file, err := os.Open("mandrill_original.png")
	if err != nil {
		fmt.Printf("Could not load baboon image: %v\n", err)
		return
	}
	defer file.Close()
	
	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Printf("Could not decode image: %v\n", err)
		return
	}
	
	// Resize for terminal aspect ratio
	resized := ResizeForTerminal(img)
	fmt.Printf("Original image: %dx%d, Resized for terminal: %dx%d\n", 
		img.Bounds().Dx(), img.Bounds().Dy(),
		resized.Bounds().Dx(), resized.Bounds().Dy())
	
	// Save resized image for inspection
	SaveDitheredImage(resized, "mandrill_resized.png")
	
	// Use Atkinson dithering (it worked better)
	atkinson := AtkinsonDither(resized)
	SaveDitheredImage(atkinson, "mandrill_atkinson.png")
	blocks := Segment8x8(atkinson)
	
	// Analyze with restricted set
	AnalyzeSegmentsRestricted(blocks, lookup, allowedChars)
	
	// Visualize
	VisualizeMatchesRestricted(blocks, lookup, allowedChars)
	
	// Save full output
	SaveFullOutputRestricted(blocks, lookup, allowedChars, "baboon_atkinson_restricted.txt")
	fmt.Println("\nSaved restricted output to baboon_atkinson_restricted.txt")
}

// ProcessColorBlocks processes image with color support
func ProcessColorBlocks(img image.Image, lookup *GlyphLookup, allowedChars map[rune]bool) [][]ColorBlock {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	
	// Calculate grid dimensions
	gridWidth := (width + 7) / 8
	gridHeight := (height + 7) / 8
	
	blocks := make([][]ColorBlock, gridHeight)
	for row := range blocks {
		blocks[row] = make([]ColorBlock, gridWidth)
	}
	
	// Process each 8x8 block
	for by := 0; by < gridHeight; by++ {
		for bx := 0; bx < gridWidth; bx++ {
			// Extract dominant colors
			fg, bg := ExtractDominantColors(img, bx, by)
			
			// Create pattern based on color proximity
			pattern := CreatePatternFromColors(img, bx, by, fg, bg)
			
			blocks[by][bx] = ColorBlock{
				Foreground: fg,
				Background: bg,
				Pattern:    pattern,
			}
		}
	}
	
	return blocks
}

// SaveColorOutput saves the output with ANSI color codes
func SaveColorOutput(blocks [][]ColorBlock, lookup *GlyphLookup, allowedChars map[rune]bool, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	for _, blockRow := range blocks {
		for _, block := range blockRow {
			// Find best matching glyph
			match := lookup.FindClosestGlyphRestricted(block.Pattern, allowedChars)
			
			// Get ANSI color codes
			fgCode := colorToANSI256(block.Foreground)
			bgCode := colorToANSI256(block.Background)
			
			// Write ANSI escape sequence
			fmt.Fprintf(file, "\x1b[38;5;%dm\x1b[48;5;%dm%c", fgCode, bgCode, match.Rune)
		}
		// Reset at end of line
		fmt.Fprintln(file, "\x1b[0m")
	}
	
	return nil
}

// RenderColorBlocksToPNG renders color blocks to a PNG image
func RenderColorBlocksToPNG(blocks [][]ColorBlock, lookup *GlyphLookup, allowedChars map[rune]bool, font *truetype.Font, outputFile string) error {
	// Create image
	width := len(blocks[0]) * 8
	height := len(blocks) * 8
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	
	// Draw each block
	for by, blockRow := range blocks {
		for bx, block := range blockRow {
			// Find matching glyph
			match := lookup.FindClosestGlyphRestricted(block.Pattern, allowedChars)
			
			// Get glyph from font
			glyphInfo := lookup.LookupRune(match.Rune)
			if glyphInfo == nil {
				continue
			}
			
			// Draw the 8x8 bitmap with colors
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					px := bx*8 + x
					py := by*8 + y
					
					// Check if bit is set in glyph bitmap
					if getBit(glyphInfo.Bitmap, x, y) {
						img.Set(px, py, block.Foreground)
					} else {
						img.Set(px, py, block.Background)
					}
				}
			}
		}
	}
	
	// Save PNG
	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer f.Close()
	
	return png.Encode(f, img)
}

// TestMandrillColor tests with color support
func TestMandrillColor() {
	// Load fonts
	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	// Check which 2x2 block characters are actually in the font
	fmt.Println("\nChecking for 2x2 block characters in font:")
	brownBlocks := []rune{' ', '▘', '▝', '▀', '▖', '▌', '▞', '▛', '▗', '▚', '▐', '▜', '▄', '▙', '▟', '█'}
	foundCount := 0
	for _, r := range brownBlocks {
		if lookup.LookupRune(r) != nil {
			fmt.Printf("  Found: %c (U+%04X)\n", r, r)
			foundCount++
		} else {
			fmt.Printf("  Missing: %c (U+%04X)\n", r, r)
		}
	}
	fmt.Printf("Found %d/%d block characters\n\n", foundCount, len(brownBlocks))
	
	// Create restricted character set
	allowedChars := CreateRestrictedCharSet()
	fmt.Printf("Using restricted character set with %d characters\n", len(allowedChars))
	
	// Load the mandrill image
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
	
	// First scale to target size for 80x40 output
	// 80 chars × 8 pixels = 640 pixels wide
	// 40 chars × 8 pixels × 2 (for aspect ratio) = 640 pixels high
	targetSize := image.NewRGBA(image.Rect(0, 0, 640, 640))
	
	// Simple nearest neighbor scaling
	xRatio := float64(img.Bounds().Dx()) / 640.0
	yRatio := float64(img.Bounds().Dy()) / 640.0
	
	for y := 0; y < 640; y++ {
		for x := 0; x < 640; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			targetSize.Set(x, y, img.At(srcX, srcY))
		}
	}
	
	// Then resize for terminal aspect ratio
	resized := ResizeForTerminal(targetSize)
	fmt.Printf("Original: %dx%d → Scaled: %dx%d → Terminal: %dx%d → Output: %dx%d chars\n", 
		img.Bounds().Dx(), img.Bounds().Dy(),
		targetSize.Bounds().Dx(), targetSize.Bounds().Dy(),
		resized.Bounds().Dx(), resized.Bounds().Dy(),
		resized.Bounds().Dx()/8, resized.Bounds().Dy()/8)
	
	// Process with color
	fmt.Println("Processing color blocks...")
	colorBlocks := ProcessColorBlocks(resized, lookup, allowedChars)
	
	// Save color output
	SaveColorOutput(colorBlocks, lookup, allowedChars, "mandrill_color.ans")
	fmt.Println("Saved color output to mandrill_color.ans")
	
	// Render to PNG
	err = RenderColorBlocksToPNG(colorBlocks, lookup, allowedChars, font, "mandrill_8x8_color.png")
	if err != nil {
		fmt.Printf("Error rendering to PNG: %v\n", err)
	} else {
		fmt.Println("Saved PNG output to mandrill_8x8_color.png")
	}
	
	// Also save a preview of first 10x10
	fmt.Println("\nFirst 10x10 color blocks (preview):")
	for y := 0; y < 10 && y < len(colorBlocks); y++ {
		for x := 0; x < 10 && x < len(colorBlocks[y]); x++ {
			block := colorBlocks[y][x]
			match := lookup.FindClosestGlyphRestricted(block.Pattern, allowedChars)
			
			fgCode := colorToANSI256(block.Foreground)
			bgCode := colorToANSI256(block.Background)
			
			fmt.Printf("\x1b[38;5;%dm\x1b[48;5;%dm%c", fgCode, bgCode, match.Rune)
		}
		fmt.Print("\x1b[0m\n")
	}
}

// Create a test pattern with various densities and patterns
func createTestPattern() image.Image {
	img := image.NewGray(image.Rect(0, 0, 256, 256))
	
	// Gradient from left to right
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			// Add some circular patterns
			cx, cy := 128.0, 128.0
			dist := math.Sqrt(math.Pow(float64(x)-cx, 2) + math.Pow(float64(y)-cy, 2))
			
			// Combine gradient and circles
			value := uint8(x)
			if dist < 40 {
				value = 255
			} else if dist < 60 {
				value = 0
			}
			
			img.SetGray(x, y, color.Gray{value})
		}
	}
	
	return img
}