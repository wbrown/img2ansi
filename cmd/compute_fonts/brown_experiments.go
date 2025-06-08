package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"time"
	"github.com/wbrown/img2ansi"
)

// ColorSelectionStrategy defines how colors are selected for each block
type ColorSelectionStrategy int

const (
	ColorStrategyDominant    ColorSelectionStrategy = iota // Extract 2 dominant colors from block
	ColorStrategyExhaustive                                 // Test all possible color pairs
	ColorStrategyKMeans                                     // K-means clustering to find colors
	ColorStrategyOptimized                                  // K-means + nearest palette colors
	ColorStrategyFrequency                                  // Most frequent palette colors in block
	ColorStrategyContrast                                   // Maximize contrast between fg/bg
	ColorStrategyQuantized                                  // Pre-quantize pixels before matching
	ColorStrategyTrueExhaustive                             // TRUE exhaustive search (all char+color combos)
)

// CharacterSetType defines which characters are available for matching
type CharacterSetType int

const (
	CharSetDensity      CharacterSetType = iota // Standard density chars (space, shades, blocks)
	CharSetPatternsOnly                          // No space or full block
	CharSetNoSpace                               // No space only
	CharSetAll                                   // All available characters
	CharSetCustom                                // User-defined character set
)

// BrownDitheringConfig configures the Brown dithering algorithm
type BrownDitheringConfig struct {
	// Color Selection
	ColorStrategy      ColorSelectionStrategy
	ColorSearchDepth   int     // For k-means/optimization strategies
	QuantizationLevels int     // For quantized strategy
	ContrastThreshold  float64 // Minimum contrast for contrast strategy
	
	// Character Set
	CharacterSet CharacterSetType
	CustomChars  []rune
	
	// Error Diffusion
	EnableDiffusion   bool
	DiffusionStrength float64 // 0.0 to 1.0
	
	// Optimization
	EnableCaching bool
	CacheThreshold float64
	MaxColorPairs int // Limit color combinations tested
	
	// Output
	PaletteSize int // 16 or 256
	OutputFile  string
	OutputPNG   string
	
	// Progress
	ShowProgress bool
	DebugMode    bool
	
	// Limits
	MaxBlocks int // Maximum blocks to process (0 = all)
}

// DefaultBrownConfig returns a standard configuration
func DefaultBrownConfig() BrownDitheringConfig {
	return BrownDitheringConfig{
		ColorStrategy:      ColorStrategyDominant,
		ColorSearchDepth:   8,
		QuantizationLevels: 8,
		ContrastThreshold:  30.0,
		CharacterSet:       CharSetDensity,
		EnableDiffusion:    false,
		DiffusionStrength:  1.0,
		EnableCaching:      false,
		CacheThreshold:     100.0,
		MaxColorPairs:      0, // 0 = no limit
		PaletteSize:        16,
		OutputFile:         OutputPath("brown_output.ans"),
		OutputPNG:          OutputPath("brown_output.png"),
		ShowProgress:       true,
		DebugMode:          false,
	}
}

// BrownDithering performs Brown dithering with the given configuration
func BrownDithering(img image.Image, lookup *GlyphLookup, config BrownDitheringConfig) error {
	startTime := time.Now()
	
	// Get character set
	chars := getCharacterSet(config.CharacterSet, config.CustomChars)
	if config.DebugMode {
		fmt.Printf("Using %d characters: %v\n", len(chars), chars)
	}
	
	// Load palette using main package's system
	paletteName := "ansi16"
	if config.PaletteSize == 256 {
		paletteName = "ansi256"
	}
	
	fgTable, _, err := img2ansi.LoadPalette(paletteName)
	if err != nil {
		return fmt.Errorf("failed to load palette: %v", err)
	}
	
	// Get RGB colors from the palette for color selection
	palette := make([]img2ansi.RGB, len(fgTable.AnsiData))
	for i, entry := range fgTable.AnsiData {
		// Convert uint32 key to RGB
		palette[i] = img2ansi.RGB{
			R: uint8((entry.Key >> 16) & 0xFF),
			G: uint8((entry.Key >> 8) & 0xFF),
			B: uint8(entry.Key & 0xFF),
		}
	}
	
	if config.DebugMode {
		fmt.Printf("Using %d color palette\n", len(palette))
	}
	
	// Create output file
	outputFile, err := os.Create(config.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()
	
	// Calculate dimensions
	bounds := img.Bounds()
	width := bounds.Dx() / 8
	height := bounds.Dy() / 8
	totalBlocks := width * height
	
	fmt.Printf("Processing %dx%d image as %dx%d blocks\n", 
		bounds.Dx(), bounds.Dy(), width, height)
	
	// Create working image for error diffusion
	var workingImg *image.RGBA
	if config.EnableDiffusion {
		workingImg = image.NewRGBA(bounds)
		// Copy original image
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				workingImg.Set(x, y, img.At(x, y))
			}
		}
		fmt.Println("Error diffusion enabled")
	}
	
	// Initialize color selector
	colorSelector := createColorSelector(config)
	
	// Initialize cache if enabled
	// TODO: Implement caching
	// var cache *ApproximateCache
	// if config.EnableCaching {
	// 	cache = NewApproximateCache()
	// 	fmt.Println("Block caching enabled")
	// }
	
	// Progress tracking
	progressTracker := CreateProgressTracker(totalBlocks)
	processed := 0
	
	// Statistics
	charCounts := make(map[rune]int)
	
	// Build BlockRune array
	blocks := make([][]img2ansi.BlockRune, height)
	blocksProcessed := 0
	for y := 0; y < height; y++ {
		blocks[y] = make([]img2ansi.BlockRune, width)
		for x := 0; x < width; x++ {
			// Check if we've hit the block limit
			if config.MaxBlocks > 0 && blocksProcessed >= config.MaxBlocks {
				// Fill remaining blocks with spaces
				blocks[y][x] = img2ansi.BlockRune{
					Rune: ' ',
					FG:   img2ansi.RGB{R: 0, G: 0, B: 0},
					BG:   img2ansi.RGB{R: 0, G: 0, B: 0},
				}
				continue
			}
			// Extract block pixels
			var sourceImg image.Image
			if config.EnableDiffusion {
				sourceImg = workingImg
			} else {
				sourceImg = img
			}
			
			blockPixels := extractBlockPixelsRGB(sourceImg, x, y)
			
			// Select color candidates
			colorPairs := colorSelector.SelectColors(blockPixels, palette)
			
			// Debug output for first block
			if config.DebugMode && x == 0 && y == 0 {
				fmt.Printf("First block: %d color pairs generated\n", len(colorPairs))
			}
			
			// Find best character and colors
			bestChar, bestFg, bestBg := findBestCharacterMatch(
				blockPixels, chars, colorPairs, lookup, config)
			
			// Apply error diffusion
			if config.EnableDiffusion && workingImg != nil {
				applyBlockErrorDiffusion(workingImg, x, y, bestChar, bestFg, bestBg, 
					lookup, config.DiffusionStrength)
			}
			
			// Store in BlockRune array
			blocks[y][x] = img2ansi.BlockRune{
				Rune: bestChar,
				FG:   bestFg,
				BG:   bestBg,
			}
			
			// Update statistics
			charCounts[bestChar]++
			processed++
			blocksProcessed++
			
			if config.ShowProgress {
				progressTracker(processed)
			}
		}
	}
	
	// Convert BlockRune array to ANSI and write to file
	ansiOutput := img2ansi.RenderToAnsi(blocks)
	outputFile.WriteString(ansiOutput)
	
	// Report results
	fmt.Printf("\nCompleted in %v\n", time.Since(startTime))
	fmt.Printf("Saved to %s\n", config.OutputFile)
	
	if config.DebugMode {
		PrintCharacterUsage(charCounts, chars, totalBlocks)
	}
	
	// if cache != nil {
	// 	fmt.Printf("\nCache performance: %.1f%% hit rate\n", cache.HitRate()*100)
	// }
	
	// Render to PNG if requested
	if config.OutputPNG != "" {
		fmt.Println("Rendering to PNG...")
		if err := RenderANSIToPNG(config.OutputFile, config.OutputPNG); err != nil {
			fmt.Printf("Error rendering PNG: %v\n", err)
		} else {
			fmt.Printf("Saved PNG to %s\n", config.OutputPNG)
		}
	}
	
	return nil
}

// ColorSelector interface for different color selection strategies
type ColorSelector interface {
	SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair
}

// ColorPair represents a foreground/background color combination
// Updated to use img2ansi.RGB to match main package's Match struct
type ColorPair struct {
	Fg, Bg img2ansi.RGB
}

// createColorSelector creates the appropriate color selector for the strategy
func createColorSelector(config BrownDitheringConfig) ColorSelector {
	switch config.ColorStrategy {
	case ColorStrategyDominant:
		return &DominantColorSelector{}
	case ColorStrategyExhaustive:
		return &ExhaustiveColorSelector{MaxPairs: config.MaxColorPairs}
	case ColorStrategyKMeans:
		return &KMeansColorSelector{K: config.ColorSearchDepth}
	case ColorStrategyOptimized:
		return &OptimizedColorSelector{K: config.ColorSearchDepth}
	case ColorStrategyFrequency:
		return &FrequencyColorSelector{TopN: config.ColorSearchDepth}
	case ColorStrategyContrast:
		return &ContrastColorSelector{MinContrast: config.ContrastThreshold}
	case ColorStrategyQuantized:
		return &QuantizedColorSelector{Levels: config.QuantizationLevels}
	case ColorStrategyTrueExhaustive:
		return &TrueExhaustiveColorSelector{}
	default:
		return &DominantColorSelector{}
	}
}

// DominantColorSelector extracts the two most dominant colors
type DominantColorSelector struct{}

func (d *DominantColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
	colors := extractBlockColorsRGB(pixels)
	
	// Find nearest palette colors
	fg := findNearestPaletteColorRGB(colors[0], palette)
	bg := findNearestPaletteColorRGB(colors[1], palette)
	
	return []ColorPair{{fg, bg}, {bg, fg}}
}

// ExhaustiveColorSelector tests all possible color pairs
type ExhaustiveColorSelector struct {
	MaxPairs int
}

func (e *ExhaustiveColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
	var pairs []ColorPair
	
	// If we have a limit and a large palette, sample intelligently
	if e.MaxPairs > 0 && len(palette) > 16 {
		// First, always include the dominant colors from the block
		colors := extractBlockColorsRGB(pixels)
		fg := findNearestPaletteColorRGB(colors[0], palette)
		bg := findNearestPaletteColorRGB(colors[1], palette)
		pairs = append(pairs, ColorPair{fg, bg}, ColorPair{bg, fg})
		
		// Then sample colors evenly across the palette
		step := len(palette) / (e.MaxPairs / 4) // Use 1/4 of budget for sampling
		if step < 1 {
			step = 1
		}
		
		// Add sampled pairs
		for i := 0; i < len(palette) && len(pairs) < e.MaxPairs/2; i += step {
			for j := i + 1; j < len(palette) && len(pairs) < e.MaxPairs/2; j += step {
				pairs = append(pairs, ColorPair{palette[i], palette[j]})
				pairs = append(pairs, ColorPair{palette[j], palette[i]})
			}
		}
		
		// Use remaining budget to test colors near the dominant colors
		for i := 0; i < len(palette) && len(pairs) < e.MaxPairs; i++ {
			// Test combinations with dominant foreground
			pairs = append(pairs, ColorPair{fg, palette[i]})
			if len(pairs) < e.MaxPairs {
				pairs = append(pairs, ColorPair{palette[i], fg})
			}
			// Test combinations with dominant background
			if len(pairs) < e.MaxPairs {
				pairs = append(pairs, ColorPair{bg, palette[i]})
			}
			if len(pairs) < e.MaxPairs {
				pairs = append(pairs, ColorPair{palette[i], bg})
			}
		}
		
		return pairs
	}
	
	// Original exhaustive approach for small palettes
	for i := 0; i < len(palette); i++ {
		for j := i + 1; j < len(palette); j++ {
			pairs = append(pairs, ColorPair{palette[i], palette[j]})
			pairs = append(pairs, ColorPair{palette[j], palette[i]})
			
			if e.MaxPairs > 0 && len(pairs) >= e.MaxPairs {
				return pairs
			}
		}
	}
	
	return pairs
}

// KMeansColorSelector uses k-means to find dominant colors
type KMeansColorSelector struct {
	K int
}

func (k *KMeansColorSelector) SelectColors(pixels []img2ansi.RGB, palette []img2ansi.RGB) []ColorPair {
	// Run k-means clustering
	centers := kmeansColorsRGB(pixels, k.K)
	
	// Find nearest palette colors for each center
	paletteColors := make([]img2ansi.RGB, len(centers))
	for i, center := range centers {
		paletteColors[i] = findNearestPaletteColorRGB(center, palette)
	}
	
	// Generate pairs from palette colors
	var pairs []ColorPair
	for i := 0; i < len(paletteColors); i++ {
		for j := i + 1; j < len(paletteColors); j++ {
			pairs = append(pairs, ColorPair{paletteColors[i], paletteColors[j]})
			pairs = append(pairs, ColorPair{paletteColors[j], paletteColors[i]})
		}
	}
	
	return pairs
}

// getCharacterSet returns the appropriate character set
func getCharacterSet(setType CharacterSetType, customChars []rune) []rune {
	switch setType {
	case CharSetDensity:
		return []rune{' ', '░', '▒', '▓', '█', '▀', '▄', '▌', '▐'}
		
	case CharSetPatternsOnly:
		return []rune{'░', '▒', '▓', '▀', '▄', '▌', '▐'}
		
	case CharSetNoSpace:
		return []rune{'░', '▒', '▓', '█', '▀', '▄', '▌', '▐'}
		
	case CharSetAll:
		// This would return all available glyphs
		return []rune{' ', '░', '▒', '▓', '█', '▀', '▄', '▌', '▐',
			'▘', '▝', '▖', '▗', '▚', '▞', '▛', '▜', '▙', '▟'}
		
	case CharSetCustom:
		return customChars
		
	default:
		return []rune{' ', '░', '▒', '▓', '█', '▀', '▄', '▌', '▐'}
	}
}


// findBestCharacterMatch finds the best character and color combination
func findBestCharacterMatch(pixels []img2ansi.RGB, chars []rune, colorPairs []ColorPair, 
	lookup *GlyphLookup, config BrownDitheringConfig) (rune, img2ansi.RGB, img2ansi.RGB) {
	
	bestChar := chars[0]
	var bestFg, bestBg img2ansi.RGB
	bestError := math.MaxFloat64
	
	// Try each character
	for _, char := range chars {
		glyphInfo := lookup.LookupRune(char)
		if glyphInfo == nil {
			continue
		}
		
		// Try each color pair
		for _, pair := range colorPairs {
			error := calculateGlyphBlockErrorRGB(pixels, glyphInfo.Bitmap, pair.Fg, pair.Bg)
			if error < bestError {
				bestError = error
				bestChar = char
				bestFg = pair.Fg
				bestBg = pair.Bg
			}
		}
	}
	
	return bestChar, bestFg, bestBg
}


// applyBlockErrorDiffusion applies error diffusion for a block
func applyBlockErrorDiffusion(img *image.RGBA, blockX, blockY int, char rune, 
	fg, bg img2ansi.RGB, lookup *GlyphLookup, strength float64) {
	
	glyphInfo := lookup.LookupRune(char)
	if glyphInfo == nil {
		return
	}
	
	// Calculate and diffuse errors for each pixel
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			px := blockX*8 + x
			py := blockY*8 + y
			
			// Get actual and target colors
			actual := img.At(px, py)
			var target color.Color
			if getBit(glyphInfo.Bitmap, x, y) {
				target = rgbToColor(fg)
			} else {
				target = rgbToColor(bg)
			}
			
			// Calculate error
			errorR, errorG, errorB := calculateColorError(actual, target)
			
			// Apply Floyd-Steinberg distribution
			distributeErrorDiffusion(img, px, py, errorR, errorG, errorB, strength)
		}
	}
}

// Cache operations will be implemented when we add the cache system

// Named configurations for common use cases
var (
	BrownConfigOriginal = BrownDitheringConfig{
		ColorStrategy: ColorStrategyDominant,
		CharacterSet:  CharSetDensity,
		PaletteSize:   16,
		OutputFile:    OutputPath("brown_original.ans"),
	}
	
	BrownConfigExhaustive = BrownDitheringConfig{
		ColorStrategy: ColorStrategyExhaustive,
		CharacterSet:  CharSetDensity,
		PaletteSize:   16,
		MaxColorPairs: 100, // Limit to prevent timeout with 256 colors
		OutputFile:    OutputPath("brown_exhaustive.ans"),
	}
	
	BrownConfigDiffusion = BrownDitheringConfig{
		ColorStrategy:    ColorStrategyExhaustive,
		CharacterSet:     CharSetDensity,
		EnableDiffusion:  true,
		PaletteSize:      16,
		MaxColorPairs:    100, // Limit to prevent timeout with 256 colors
		OutputFile:       OutputPath("brown_diffusion.ans"),
	}
	
	BrownConfigOptimized = BrownDitheringConfig{
		ColorStrategy:    ColorStrategyOptimized,
		ColorSearchDepth: 8,
		CharacterSet:     CharSetDensity,
		EnableCaching:    true,
		PaletteSize:      16,
		OutputFile:       OutputPath("brown_optimized.ans"),
	}
	
	BrownConfigPatternsOnly = BrownDitheringConfig{
		ColorStrategy:   ColorStrategyContrast,
		CharacterSet:    CharSetPatternsOnly,
		EnableDiffusion: true,
		PaletteSize:     16,
		OutputFile:      OutputPath("brown_patterns.ans"),
	}
	
	BrownConfigTrueExhaustive = BrownDitheringConfig{
		ColorStrategy:   ColorStrategyTrueExhaustive,
		CharacterSet:    CharSetDensity,
		PaletteSize:     16,
		OutputFile:      OutputPath("brown_true_exhaustive_full.ans"),
		MaxBlocks:       0, // Process all blocks
		ShowProgress:    true,
		DebugMode:       true,
	}
)

// RunBrownExperiment runs a Brown dithering experiment with the given config name
func RunBrownExperiment(configName string) error {
	return RunBrownExperimentWithPalette(configName, 16)
}

// RunBrownExperimentWithPalette runs a Brown dithering experiment with specified palette size
func RunBrownExperimentWithPalette(configName string, paletteSize int) error {
	// Load image
	resized, err := LoadMandrillImage()
	if err != nil {
		return err
	}
	
	// Load fonts
	lookup := LoadFontGlyphs()
	
	// Get configuration
	var config BrownDitheringConfig
	switch configName {
	case "original":
		config = BrownConfigOriginal
	case "exhaustive":
		config = BrownConfigExhaustive
	case "diffusion":
		config = BrownConfigDiffusion
	case "optimized":
		config = BrownConfigOptimized
	case "patterns":
		config = BrownConfigPatternsOnly
	case "true-exhaustive-full":
		config = BrownConfigTrueExhaustive
	case "quantized":
		config = DefaultBrownConfig()
		config.ColorStrategy = ColorStrategyQuantized
		config.OutputFile = OutputPath("brown_quantized.ans")
	case "smart":
		config = DefaultBrownConfig()
		config.ColorStrategy = ColorStrategyDominant
		config.EnableDiffusion = true
		config.OutputFile = OutputPath("brown_smart.ans")
	case "kmeans":
		config = DefaultBrownConfig()
		config.ColorStrategy = ColorStrategyKMeans
		config.ColorSearchDepth = 4
		config.EnableDiffusion = true
		config.OutputFile = OutputPath("brown_kmeans.ans")
	case "compare":
		config = DefaultBrownConfig()
		config.ColorStrategy = ColorStrategyFrequency
		config.ColorSearchDepth = 8
		config.OutputFile = OutputPath("brown_compare.ans")
	case "contrast":
		config = DefaultBrownConfig()
		config.ColorStrategy = ColorStrategyContrast
		config.EnableDiffusion = true
		config.OutputFile = OutputPath("brown_contrast.ans")
	case "no-space":
		config = DefaultBrownConfig()
		config.ColorStrategy = ColorStrategyDominant
		config.CharacterSet = CharSetNoSpace
		config.OutputFile = OutputPath("brown_no_space.ans")
	default:
		// Try to parse as a custom config
		config = DefaultBrownConfig()
		config.OutputFile = OutputPath(fmt.Sprintf("brown_%s.ans", configName))
	}
	
	// Override palette size
	config.PaletteSize = paletteSize
	
	// Update output filename to include palette size
	if paletteSize == 256 {
		// Replace .ans with _256.ans
		config.OutputFile = config.OutputFile[:len(config.OutputFile)-4] + "_256.ans"
		
		// Scale up MaxColorPairs for 256-color palette if using exhaustive strategy
		if config.ColorStrategy == ColorStrategyExhaustive && config.MaxColorPairs > 0 {
			// For 256 colors, we need more pairs to get good coverage
			// But not too many to avoid timeout
			config.MaxColorPairs = 1000
		}
	}
	
	fmt.Printf("Running Brown experiment: %s (palette: %d colors)\n", configName, paletteSize)
	return BrownDithering(resized, lookup, config)
}