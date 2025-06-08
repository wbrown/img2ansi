package main

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"time"
	"github.com/wbrown/img2ansi"
)

// colorDistance uses the img2ansi color distance calculation
// This is a compatibility wrapper - new code should use RGB.ColorDistance directly
func colorDistance(c1, c2 color.Color) float64 {
	return colorToRGB(c1).ColorDistance(colorToRGB(c2))
}

// rgbToANSI256 converts RGB to ANSI 256 color code
// Removed rgbToANSI256 - use RGB24ToANSI256 from color_utils.go instead

// calculateBlockError calculates total pixel error for a character
func calculateBlockError(pixels []color.Color, charBitmap GlyphBitmap, fg, bg color.Color) float64 {
	// Convert to RGB and use the RGB version
	rgbPixels := make([]img2ansi.RGB, len(pixels))
	for i, p := range pixels {
		rgbPixels[i] = colorToRGB(p)
	}
	return calculateGlyphBlockErrorRGB(rgbPixels, charBitmap, colorToRGB(fg), colorToRGB(bg))
}

// extractBlockColors extracts two most common colors from pixel array
func extractBlockColors(pixels []color.Color) []color.Color {
	// Use ExtractTopColors from color_utils.go
	colors := ExtractTopColors(pixels, 2)
	
	// Ensure we have at least 2 colors
	for len(colors) < 2 {
		colors = append(colors, color.Black)
	}
	
	return colors
}

// writeANSIChar has been removed. Use ProcessImageBlocksToBlockRunes 
// and img2ansi.RenderToAnsi instead.

// Common character sets used across tests
var (
	// DensityChars includes all density-based characters for 8x8 rendering
	DensityChars = []rune{
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
	
	// PatternsOnly excludes space and full block to force pattern usage
	PatternsOnly = []rune{
		'░',  // 25%
		'▒',  // 50%
		'▓',  // 75%
		'▀',  // top half
		'▄',  // bottom half
		'▌',  // left half
		'▐',  // right half
	}
)

// LoadMandrillImage loads and prepares the standard test image
func LoadMandrillImage() (*image.RGBA, error) {
	file, err := os.Open("mandrill_original.png")
	if err != nil {
		return nil, fmt.Errorf("could not load mandrill image: %v", err)
	}
	defer file.Close()
	
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("could not decode image: %v", err)
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
	
	return resized, nil
}

// LoadFontGlyphs loads the standard fonts and returns the glyph lookup
func LoadFontGlyphs() *GlyphLookup {
	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	glyphs := analyzeFont(font, safeFont)
	return NewGlyphLookup(glyphs)
}

// ExtractBlockPixels extracts an 8x8 pixel block from an image
func ExtractBlockPixels(img *image.RGBA, blockX, blockY int) []color.Color {
	blockPixels := make([]color.Color, 64)
	for dy := 0; dy < 8; dy++ {
		for dx := 0; dx < 8; dx++ {
			px := blockX*8 + dx
			py := blockY*8 + dy
			blockPixels[dy*8+dx] = img.At(px, py)
		}
	}
	return blockPixels
}


// ProcessImageBlocks processes an image in 8x8 blocks with a callback function
type BlockProcessor func(blockX, blockY int, pixels []color.Color) (char rune, fg, bg color.Color)

// BlockProcessorRGB processes blocks using RGB types
type BlockProcessorRGB func(blockX, blockY int, pixels []img2ansi.RGB) (char rune, fg, bg img2ansi.RGB)

func ProcessImageBlocks(img *image.RGBA, processor BlockProcessor, output io.Writer, palette []color.Color) map[rune]int {
	// Convert to RGB types
	rgbPalette := colorsToRGBs(palette)
	
	// Create RGB processor wrapper
	rgbProcessor := func(blockX, blockY int, pixels []img2ansi.RGB) (char rune, fg, bg img2ansi.RGB) {
		// Convert pixels to color.Color
		colorPixels := rgbsToColors(pixels)
		char, fgColor, bgColor := processor(blockX, blockY, colorPixels)
		return char, colorToRGB(fgColor), colorToRGB(bgColor)
	}
	
	return ProcessImageBlocksRGB(img, rgbProcessor, output, rgbPalette)
}

// ProcessImageBlocksRGB processes an image using RGB types
func ProcessImageBlocksRGB(img *image.RGBA, processor BlockProcessorRGB, output io.Writer, palette []img2ansi.RGB) map[rune]int {
	// Build blocks first
	blocks := ProcessImageBlocksToBlockRunes(img, processor)
	
	// Convert to ANSI and write
	ansiOutput := img2ansi.RenderToAnsi(blocks)
	output.Write([]byte(ansiOutput))
	
	// Count characters for statistics
	charCounts := make(map[rune]int)
	for _, row := range blocks {
		for _, block := range row {
			charCounts[block.Rune]++
		}
	}
	
	return charCounts
}

// ProcessImageBlocksToBlockRunes processes an image and returns BlockRune array
func ProcessImageBlocksToBlockRunes(img *image.RGBA, processor BlockProcessorRGB) [][]img2ansi.BlockRune {
	height := 40  // 320 pixels / 8
	width := 80   // 640 pixels / 8
	
	blocks := make([][]img2ansi.BlockRune, height)
	for y := 0; y < height; y++ {
		blocks[y] = make([]img2ansi.BlockRune, width)
		for x := 0; x < width; x++ {
			blockPixels := extractBlockPixelsRGB(img.SubImage(img.Bounds()).(*image.RGBA), x, y)
			char, fg, bg := processor(x, y, blockPixels)
			blocks[y][x] = img2ansi.BlockRune{
				Rune: char,
				FG:   fg,
				BG:   bg,
			}
		}
	}
	
	return blocks
}

// PrintCharacterUsage prints character usage statistics
func PrintCharacterUsage(charCounts map[rune]int, chars []rune, total int) {
	fmt.Println("\nCharacter usage:")
	for _, char := range chars {
		count := charCounts[char]
		percent := float64(count) * 100.0 / float64(total)
		if char == ' ' {
			fmt.Printf("Space: %d (%.1f%%)\n", count, percent)
		} else {
			fmt.Printf("'%c': %d (%.1f%%)\n", char, count, percent)
		}
	}
}

// RenderAndSave is now in render.go

// CreateProgressTracker returns a function that tracks and displays progress
func CreateProgressTracker(totalBlocks int) func(processed int) {
	startTime := time.Now()
	return func(processed int) {
		if processed%100 == 0 || processed == totalBlocks {
			elapsed := time.Since(startTime)
			perBlock := elapsed / time.Duration(processed)
			remaining := perBlock * time.Duration(totalBlocks-processed)
			fmt.Printf("\rProgress: %d/%d blocks (%.1f%%) - Est. remaining: %v",
				processed, totalBlocks, float64(processed)/float64(totalBlocks)*100, remaining)
			if processed == totalBlocks {
				fmt.Println()
			}
		}
	}
}

// FindBestCharacterAndColors finds the best character and color combination for a block
func FindBestCharacterAndColors(blockPixels []color.Color, chars []rune, lookup *GlyphLookup, palette []color.Color) (rune, color.Color, color.Color, float64) {
	// Convert to RGB and use RGB version
	rgbPixels := make([]img2ansi.RGB, len(blockPixels))
	for i, p := range blockPixels {
		rgbPixels[i] = colorToRGB(p)
	}
	rgbPalette := colorsToRGBs(palette)
	
	char, fg, bg, err := FindBestCharacterAndColorsRGB(rgbPixels, chars, lookup, rgbPalette)
	return char, rgbToColor(fg), rgbToColor(bg), err
}

// FindBestCharacterAndColorsRGB finds the best character and color combination using RGB types
func FindBestCharacterAndColorsRGB(blockPixels []img2ansi.RGB, chars []rune, lookup *GlyphLookup, palette []img2ansi.RGB) (rune, img2ansi.RGB, img2ansi.RGB, float64) {
	bestChar := chars[0]
	bestError := 1e9
	var bestFg, bestBg img2ansi.RGB
	
	// Extract dominant colors from block
	colors := extractBlockColorsRGB(blockPixels)
	if len(colors) < 2 {
		return ' ', img2ansi.RGB{0, 0, 0}, img2ansi.RGB{255, 255, 255}, bestError
	}
	
	for _, char := range chars {
		charInfo := lookup.LookupRune(char)
		if charInfo == nil {
			continue
		}
		
		// Try both orientations
		error1 := calculateGlyphBlockErrorRGB(blockPixels, charInfo.Bitmap, colors[0], colors[1])
		if error1 < bestError {
			bestError = error1
			bestChar = char
			bestFg = colors[0]
			bestBg = colors[1]
		}
		
		error2 := calculateGlyphBlockErrorRGB(blockPixels, charInfo.Bitmap, colors[1], colors[0])
		if error2 < bestError {
			bestError = error2
			bestChar = char
			bestFg = colors[1]
			bestBg = colors[0]
		}
	}
	
	return bestChar, bestFg, bestBg, bestError
}

// CreateANSI256Palette is deprecated and no longer used
// Use img2ansi.LoadPalette("ansi256") instead

// ProcessImageBlocksWithProgress processes blocks with progress tracking
func ProcessImageBlocksWithProgress(img *image.RGBA, processor BlockProcessor, output io.Writer, palette []color.Color) map[rune]int {
	// Convert to RGB types
	rgbPalette := colorsToRGBs(palette)
	
	// Create RGB processor wrapper
	rgbProcessor := func(blockX, blockY int, pixels []img2ansi.RGB) (char rune, fg, bg img2ansi.RGB) {
		// Convert pixels to color.Color
		colorPixels := rgbsToColors(pixels)
		char, fgColor, bgColor := processor(blockX, blockY, colorPixels)
		return char, colorToRGB(fgColor), colorToRGB(bgColor)
	}
	
	return ProcessImageBlocksWithProgressRGB(img, rgbProcessor, output, rgbPalette)
}

// ProcessImageBlocksWithProgressRGB processes blocks with progress tracking using RGB types
func ProcessImageBlocksWithProgressRGB(img *image.RGBA, processor BlockProcessorRGB, output io.Writer, palette []img2ansi.RGB) map[rune]int {
	// Build blocks with progress tracking
	blocks := ProcessImageBlocksToBlockRunesWithProgress(img, processor)
	
	// Convert to ANSI and write
	ansiOutput := img2ansi.RenderToAnsi(blocks)
	output.Write([]byte(ansiOutput))
	
	// Count characters for statistics
	charCounts := make(map[rune]int)
	for _, row := range blocks {
		for _, block := range row {
			charCounts[block.Rune]++
		}
	}
	
	return charCounts
}

// ProcessImageBlocksToBlockRunesWithProgress processes an image with progress tracking
func ProcessImageBlocksToBlockRunesWithProgress(img *image.RGBA, processor BlockProcessorRGB) [][]img2ansi.BlockRune {
	height := 40  // 320 pixels / 8
	width := 80   // 640 pixels / 8
	totalBlocks := height * width
	processed := 0
	
	tracker := CreateProgressTracker(totalBlocks)
	
	blocks := make([][]img2ansi.BlockRune, height)
	for y := 0; y < height; y++ {
		blocks[y] = make([]img2ansi.BlockRune, width)
		for x := 0; x < width; x++ {
			blockPixels := extractBlockPixelsRGB(img.SubImage(img.Bounds()).(*image.RGBA), x, y)
			char, fg, bg := processor(x, y, blockPixels)
			blocks[y][x] = img2ansi.BlockRune{
				Rune: char,
				FG:   fg,
				BG:   bg,
			}
			
			processed++
			tracker(processed)
		}
	}
	
	return blocks
}

// BrownBlockProcessor creates a standard Brown-style block processor
func BrownBlockProcessor(chars []rune, lookup *GlyphLookup, palette []color.Color) BlockProcessor {
	return func(blockX, blockY int, pixels []color.Color) (rune, color.Color, color.Color) {
		char, fg, bg, _ := FindBestCharacterAndColors(pixels, chars, lookup, palette)
		return char, fg, bg
	}
}

// BrownBlockProcessorRGB creates a standard Brown-style block processor using RGB types
func BrownBlockProcessorRGB(chars []rune, lookup *GlyphLookup, palette []img2ansi.RGB) BlockProcessorRGB {
	return func(blockX, blockY int, pixels []img2ansi.RGB) (rune, img2ansi.RGB, img2ansi.RGB) {
		char, fg, bg, _ := FindBestCharacterAndColorsRGB(pixels, chars, lookup, palette)
		return char, fg, bg
	}
}

// StandardBrownTest runs a standard Brown-style test with given parameters
func StandardBrownTest(testName, outputName string, chars []rune, use256Colors bool) {
	lookup := LoadFontGlyphs()
	resized, err := LoadMandrillImage()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	// Load palette from main package
	paletteName := "ansi16"
	if use256Colors {
		paletteName = "ansi256"
	}
	
	fgTable, _, err := img2ansi.LoadPalette(paletteName)
	if err != nil {
		fmt.Printf("Error loading palette: %v\n", err)
		return
	}
	
	// Extract colors from palette
	palette := make([]color.Color, len(fgTable.AnsiData))
	for i, entry := range fgTable.AnsiData {
		palette[i] = color.RGBA{
			R: uint8((entry.Key >> 16) & 0xFF),
			G: uint8((entry.Key >> 8) & 0xFF),
			B: uint8(entry.Key & 0xFF),
			A: 255,
		}
	}
	
	fmt.Printf("=== %s ===\n", testName)
	
	ansFile := outputName + ".ans"
	pngFile := outputName + ".png"
	
	outputFile, _ := os.Create(ansFile)
	defer outputFile.Close()
	
	processor := BrownBlockProcessor(chars, lookup, palette)
	charCounts := ProcessImageBlocks(resized, processor, outputFile, palette)
	
	fmt.Printf("\nSaved to %s\n", ansFile)
	RenderAndSave(ansFile, pngFile)
	PrintCharacterUsage(charCounts, chars, 80*40)
}

// DistributeError8x8 distributes error to neighboring blocks for error diffusion
func DistributeError8x8(working *image.RGBA, blockX, blockY int, errorR, errorG, errorB float64) {
	// Floyd-Steinberg coefficients for 8x8 blocks
	// Right: 7/16, Bottom-left: 3/16, Bottom: 5/16, Bottom-right: 1/16
	distributions := []struct {
		dx, dy int
		factor float64
	}{
		{1, 0, 7.0 / 16.0},   // Right
		{-1, 1, 3.0 / 16.0},  // Bottom-left
		{0, 1, 5.0 / 16.0},   // Bottom
		{1, 1, 1.0 / 16.0},   // Bottom-right
	}
	
	for _, dist := range distributions {
		newX := blockX + dist.dx
		newY := blockY + dist.dy
		
		// Distribute to all pixels in the neighboring block
		for dy := 0; dy < 8; dy++ {
			for dx := 0; dx < 8; dx++ {
				px := newX*8 + dx
				py := newY*8 + dy
				
				if px >= 0 && px < working.Bounds().Dx() && py >= 0 && py < working.Bounds().Dy() {
					c := working.RGBAAt(px, py)
					newR := float64(c.R) + errorR*dist.factor
					newG := float64(c.G) + errorG*dist.factor
					newB := float64(c.B) + errorB*dist.factor
					
					// Clamp values
					if newR < 0 { newR = 0 }
					if newR > 255 { newR = 255 }
					if newG < 0 { newG = 0 }
					if newG > 255 { newG = 255 }
					if newB < 0 { newB = 0 }
					if newB > 255 { newB = 255 }
					
					working.SetRGBA(px, py, color.RGBA{
						R: uint8(newR),
						G: uint8(newG),
						B: uint8(newB),
						A: 255,
					})
				}
			}
		}
	}
}