package main

import (
	"fmt"
	"os"
	"github.com/wbrown/img2ansi"
)

// VerifyExperiments is a simple program to check experiment differences
func VerifyExperiments() {
	// Print character sets
	fmt.Println("=== Character Sets ===")
	sets := map[string]CharacterSetType{
		"Density (default)": CharSetDensity,
		"PatternsOnly":      CharSetPatternsOnly,
		"NoSpace":           CharSetNoSpace,
		"All":               CharSetAll,
	}
	
	for name, setType := range sets {
		chars := getCharacterSet(setType, nil)
		fmt.Printf("\n%s (%d chars): %v\n", name, len(chars), string(chars))
	}
	
	// Print experiment configurations
	fmt.Println("\n=== Experiment Configurations ===")
	experiments := []struct{
		name string
		configName string
	}{
		{"style", "original"},
		{"quantized", "quantized"},
		{"diffusion", "diffusion"}, 
		{"smart-diffusion", "smart"},
		{"kmeans-diffusion", "kmeans"},
		{"compare-diffusion", "compare"},
		{"contrast-diffusion", "contrast"},
		{"no-space", "no-space"},
		{"patterns-only", "patterns"},
		{"optimized", "optimized"},
		{"exhaustive", "exhaustive"},
	}
	
	for _, exp := range experiments {
		// Simulate getting the config
		var colorStrategy string
		var charSet string
		var diffusion bool
		
		switch exp.configName {
		case "original":
			colorStrategy = "Dominant"
			charSet = "Density"
			diffusion = false
		case "quantized":
			colorStrategy = "Quantized"
			charSet = "Density"
			diffusion = false
		case "diffusion":
			colorStrategy = "Exhaustive"
			charSet = "Density"
			diffusion = true
		case "smart":
			colorStrategy = "Dominant"
			charSet = "Density"
			diffusion = true
		case "kmeans":
			colorStrategy = "KMeans"
			charSet = "Density"
			diffusion = true
		case "compare":
			colorStrategy = "Frequency"
			charSet = "Density"
			diffusion = false
		case "contrast":
			colorStrategy = "Contrast"
			charSet = "Density"
			diffusion = true
		case "no-space":
			colorStrategy = "Dominant"
			charSet = "NoSpace"
			diffusion = false
		case "patterns":
			colorStrategy = "Contrast"
			charSet = "PatternsOnly"
			diffusion = true
		case "optimized":
			colorStrategy = "Optimized"
			charSet = "Density"
			diffusion = false
		case "exhaustive":
			colorStrategy = "Exhaustive"
			charSet = "Density"
			diffusion = false
		}
		
		fmt.Printf("\n%-20s: Color=%s, CharSet=%s, Diffusion=%v\n", 
			exp.name, colorStrategy, charSet, diffusion)
	}
	
	// Test color selection differences
	fmt.Println("\n=== Testing Color Selection ===")
	testColorSelection()
}

func testColorSelection() {
	// Create test pixels - a mix of colors
	pixels := []img2ansi.RGB{
		{R: 255, G: 0, B: 0},   // Red
		{R: 0, G: 255, B: 0},   // Green  
		{R: 0, G: 0, B: 255},   // Blue
		{R: 255, G: 255, B: 0}, // Yellow
		{R: 128, G: 128, B: 128}, // Gray
		{R: 0, G: 0, B: 0},     // Black
		{R: 255, G: 255, B: 255}, // White
		{R: 128, G: 0, B: 128}, // Purple
	}
	
	// Simple 16-color palette
	palette := make([]img2ansi.RGB, 16)
	// Black
	palette[0] = img2ansi.RGB{R: 0, G: 0, B: 0}
	// Dark colors
	palette[1] = img2ansi.RGB{R: 128, G: 0, B: 0}
	palette[2] = img2ansi.RGB{R: 0, G: 128, B: 0}
	palette[3] = img2ansi.RGB{R: 128, G: 128, B: 0}
	palette[4] = img2ansi.RGB{R: 0, G: 0, B: 128}
	palette[5] = img2ansi.RGB{R: 128, G: 0, B: 128}
	palette[6] = img2ansi.RGB{R: 0, G: 128, B: 128}
	palette[7] = img2ansi.RGB{R: 192, G: 192, B: 192}
	// Bright colors
	palette[8] = img2ansi.RGB{R: 128, G: 128, B: 128}
	palette[9] = img2ansi.RGB{R: 255, G: 0, B: 0}
	palette[10] = img2ansi.RGB{R: 0, G: 255, B: 0}
	palette[11] = img2ansi.RGB{R: 255, G: 255, B: 0}
	palette[12] = img2ansi.RGB{R: 0, G: 0, B: 255}
	palette[13] = img2ansi.RGB{R: 255, G: 0, B: 255}
	palette[14] = img2ansi.RGB{R: 0, G: 255, B: 255}
	palette[15] = img2ansi.RGB{R: 255, G: 255, B: 255}
	
	// Test different selectors
	fmt.Println("\nDominant selector:")
	dominant := &DominantColorSelector{}
	pairs := dominant.SelectColors(pixels, palette)
	fmt.Printf("  Returns %d pairs, first: FG(%d,%d,%d) BG(%d,%d,%d)\n",
		len(pairs), pairs[0].Fg.R, pairs[0].Fg.G, pairs[0].Fg.B,
		pairs[0].Bg.R, pairs[0].Bg.G, pairs[0].Bg.B)
	
	fmt.Println("\nExhaustive selector (limited to 10):")
	exhaustive := &ExhaustiveColorSelector{MaxPairs: 10}
	pairs = exhaustive.SelectColors(pixels, palette)
	fmt.Printf("  Returns %d pairs, first: FG(%d,%d,%d) BG(%d,%d,%d)\n",
		len(pairs), pairs[0].Fg.R, pairs[0].Fg.G, pairs[0].Fg.B,
		pairs[0].Bg.R, pairs[0].Bg.G, pairs[0].Bg.B)
	
	fmt.Println("\nQuantized selector:")
	quantized := &QuantizedColorSelector{Levels: 4}
	pairs = quantized.SelectColors(pixels, palette)
	fmt.Printf("  Returns %d pairs\n", len(pairs))
}

// Run this as a separate program
func init() {
	// Only run if called directly (not during tests)
	if len(os.Args) > 1 && os.Args[1] == "verify" {
		VerifyExperiments()
		os.Exit(0)
	}
}