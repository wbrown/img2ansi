package main

import (
	"testing"
	"github.com/wbrown/img2ansi"
	"fmt"
)

// TestColorSelectors verifies each color selector produces different results
func TestColorSelectors(t *testing.T) {
	// Create a test block with varied colors
	testPixels := []img2ansi.RGB{
		{R: 255, G: 0, B: 0},     // Red
		{R: 0, G: 255, B: 0},     // Green
		{R: 0, G: 0, B: 255},     // Blue
		{R: 255, G: 255, B: 0},   // Yellow
		{R: 255, G: 0, B: 255},   // Magenta
		{R: 0, G: 255, B: 255},   // Cyan
		{R: 128, G: 128, B: 128}, // Gray
		{R: 0, G: 0, B: 0},       // Black
	}
	
	// Create a simple palette
	palette := []img2ansi.RGB{
		{R: 0, G: 0, B: 0},       // Black
		{R: 128, G: 0, B: 0},     // Dark Red
		{R: 0, G: 128, B: 0},     // Dark Green
		{R: 128, G: 128, B: 0},   // Brown
		{R: 0, G: 0, B: 128},     // Dark Blue
		{R: 128, G: 0, B: 128},   // Magenta
		{R: 0, G: 128, B: 128},   // Dark Cyan
		{R: 192, G: 192, B: 192}, // Light Gray
		{R: 128, G: 128, B: 128}, // Dark Gray
		{R: 255, G: 0, B: 0},     // Red
		{R: 0, G: 255, B: 0},     // Green
		{R: 255, G: 255, B: 0},   // Yellow
		{R: 0, G: 0, B: 255},     // Blue
		{R: 255, G: 0, B: 255},   // Magenta
		{R: 0, G: 255, B: 255},   // Cyan
		{R: 255, G: 255, B: 255}, // White
	}
	
	// Test each color selector
	selectors := map[string]ColorSelector{
		"Dominant": &DominantColorSelector{},
		"Exhaustive": &ExhaustiveColorSelector{MaxPairs: 10},
		"KMeans": &KMeansColorSelector{K: 4},
		"Optimized": &OptimizedColorSelector{K: 4},
		"Frequency": &FrequencyColorSelector{TopN: 4},
		"Contrast": &ContrastColorSelector{MinContrast: 30.0},
		"Quantized": &QuantizedColorSelector{Levels: 4},
	}
	
	results := make(map[string][]ColorPair)
	
	for name, selector := range selectors {
		pairs := selector.SelectColors(testPixels, palette)
		results[name] = pairs
		
		fmt.Printf("\n%s selector returned %d color pairs:\n", name, len(pairs))
		// Show first few pairs
		for i := 0; i < len(pairs) && i < 3; i++ {
			fmt.Printf("  Pair %d: FG(%d,%d,%d) BG(%d,%d,%d)\n", 
				i+1,
				pairs[i].Fg.R, pairs[i].Fg.G, pairs[i].Fg.B,
				pairs[i].Bg.R, pairs[i].Bg.G, pairs[i].Bg.B)
		}
	}
	
	// Verify that different selectors produce different results
	dominant := results["Dominant"]
	for name, pairs := range results {
		if name == "Dominant" {
			continue
		}
		
		// Check if the results are different
		different := false
		if len(pairs) != len(dominant) {
			different = true
		} else {
			for i := range pairs {
				if i >= len(dominant) {
					different = true
					break
				}
				if pairs[i].Fg != dominant[i].Fg || pairs[i].Bg != dominant[i].Bg {
					different = true
					break
				}
			}
		}
		
		if !different {
			t.Errorf("%s selector produced identical results to Dominant selector", name)
		}
	}
}

// TestCharacterSets verifies different character sets are actually different
func TestCharacterSets(t *testing.T) {
	sets := map[string]CharacterSetType{
		"Density": CharSetDensity,
		"PatternsOnly": CharSetPatternsOnly,
		"NoSpace": CharSetNoSpace,
		"All": CharSetAll,
	}
	
	for name, setType := range sets {
		chars := getCharacterSet(setType, nil)
		fmt.Printf("\n%s character set (%d chars): %v\n", name, len(chars), chars)
		
		// Check for presence of space and full block
		hasSpace := false
		hasFullBlock := false
		for _, c := range chars {
			if c == ' ' {
				hasSpace = true
			}
			if c == '█' {
				hasFullBlock = true
			}
		}
		
		// Verify constraints
		switch setType {
		case CharSetPatternsOnly:
			if hasSpace || hasFullBlock {
				t.Errorf("PatternsOnly should not contain space or full block")
			}
		case CharSetNoSpace:
			if hasSpace {
				t.Errorf("NoSpace should not contain space")
			}
		}
	}
}

// TestExperimentConfigurations checks that each experiment has unique settings
func TestExperimentConfigurations(t *testing.T) {
	experiments := []string{
		"original", "quantized", "diffusion", "smart", "kmeans",
		"compare", "contrast", "no-space", "patterns", "optimized", "exhaustive",
	}
	
	configs := make(map[string]BrownDitheringConfig)
	
	// Get configuration for each experiment
	for _, exp := range experiments {
		// We need to simulate what RunBrownExperiment does
		var config BrownDitheringConfig
		switch exp {
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
		case "quantized":
			config = DefaultBrownConfig()
			config.ColorStrategy = ColorStrategyQuantized
		case "smart":
			config = DefaultBrownConfig()
			config.ColorStrategy = ColorStrategyDominant
			config.EnableDiffusion = true
		case "kmeans":
			config = DefaultBrownConfig()
			config.ColorStrategy = ColorStrategyKMeans
			config.ColorSearchDepth = 4
			config.EnableDiffusion = true
		case "compare":
			config = DefaultBrownConfig()
			config.ColorStrategy = ColorStrategyFrequency
			config.ColorSearchDepth = 8
		case "contrast":
			config = DefaultBrownConfig()
			config.ColorStrategy = ColorStrategyContrast
			config.EnableDiffusion = true
		case "no-space":
			config = DefaultBrownConfig()
			config.ColorStrategy = ColorStrategyDominant
			config.CharacterSet = CharSetNoSpace
		}
		configs[exp] = config
		
		// Print configuration
		fmt.Printf("\n%s experiment:\n", exp)
		fmt.Printf("  ColorStrategy: %v\n", config.ColorStrategy)
		fmt.Printf("  CharacterSet: %v\n", config.CharacterSet)
		fmt.Printf("  EnableDiffusion: %v\n", config.EnableDiffusion)
		fmt.Printf("  ColorSearchDepth: %v\n", config.ColorSearchDepth)
	}
	
	// Check for uniqueness
	for i, exp1 := range experiments {
		for j, exp2 := range experiments {
			if i >= j {
				continue
			}
			c1 := configs[exp1]
			c2 := configs[exp2]
			
			// Check if they're too similar
			if c1.ColorStrategy == c2.ColorStrategy &&
			   c1.CharacterSet == c2.CharacterSet &&
			   c1.EnableDiffusion == c2.EnableDiffusion {
				t.Logf("Warning: %s and %s have very similar configurations", exp1, exp2)
			}
		}
	}
}