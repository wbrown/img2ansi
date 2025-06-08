package main

import (
	"fmt"
	"github.com/wbrown/img2ansi"
)

// CompareExhaustiveApproaches demonstrates the difference between the two exhaustive approaches
func CompareExhaustiveApproaches() {
	fmt.Println("=== Comparing Exhaustive Approaches ===")
	fmt.Println()
	
	// Create a test block with varied colors
	testPixels := []img2ansi.RGB{
		{R: 200, G: 50, B: 50},   // Reddish
		{R: 50, G: 200, B: 50},   // Greenish
		{R: 50, G: 50, B: 200},   // Blueish
		{R: 200, G: 200, B: 50},  // Yellowish
		{R: 100, G: 100, B: 100}, // Gray
		{R: 150, G: 75, B: 150},  // Purple
		{R: 75, G: 150, B: 150},  // Cyan
		{R: 225, G: 175, B: 125}, // Beige
	}
	
	// Simple 16-color palette
	palette := []img2ansi.RGB{
		{R: 0, G: 0, B: 0},       // Black
		{R: 170, G: 0, B: 0},     // Red
		{R: 0, G: 170, B: 0},     // Green
		{R: 170, G: 85, B: 0},    // Brown
		{R: 0, G: 0, B: 170},     // Blue
		{R: 170, G: 0, B: 170},   // Magenta
		{R: 0, G: 170, B: 170},   // Cyan
		{R: 170, G: 170, B: 170}, // Light Gray
		{R: 85, G: 85, B: 85},    // Dark Gray
		{R: 255, G: 85, B: 85},   // Light Red
		{R: 85, G: 255, B: 85},   // Light Green
		{R: 255, G: 255, B: 85},  // Yellow
		{R: 85, G: 85, B: 255},   // Light Blue
		{R: 255, G: 85, B: 255},  // Light Magenta
		{R: 85, G: 255, B: 255},  // Light Cyan
		{R: 255, G: 255, B: 255}, // White
	}
	
	// Character set
	chars := getCharacterSet(CharSetDensity, nil)
	
	fmt.Println("1. Current 'Exhaustive' Approach (ExhaustiveColorSelector):")
	fmt.Println("   - Pre-generates color pairs based on analysis")
	fmt.Println("   - Tests each character against pre-selected pairs")
	fmt.Printf("   - Total combinations tested: %d chars × %d pairs\n", len(chars), 10)
	fmt.Println()
	
	// Run current exhaustive
	exhaustive := &ExhaustiveColorSelector{MaxPairs: 10}
	pairs := exhaustive.SelectColors(testPixels, palette)
	fmt.Printf("   Generated %d color pairs:\n", len(pairs))
	for i, pair := range pairs[:3] { // Show first 3
		fmt.Printf("     Pair %d: FG(%3d,%3d,%3d) BG(%3d,%3d,%3d)\n",
			i+1, pair.Fg.R, pair.Fg.G, pair.Fg.B,
			pair.Bg.R, pair.Bg.G, pair.Bg.B)
	}
	
	fmt.Println()
	fmt.Println("2. True Exhaustive Approach:")
	fmt.Println("   - Tests EVERY character with EVERY color combination")
	fmt.Printf("   - Total combinations tested: %d chars × %d × %d colors = %d\n",
		len(chars), len(palette), len(palette), len(chars)*len(palette)*len(palette))
	fmt.Println()
	
	// Calculate what true exhaustive would find
	fmt.Println("   True exhaustive would test combinations like:")
	count := 0
	for _, char := range chars[:3] { // Show first 3 chars
		for _, fg := range palette[:2] { // Show first 2 FG colors
			for _, bg := range palette[:2] { // Show first 2 BG colors
				count++
				fmt.Printf("     %d. Char '%c' with FG(%3d,%3d,%3d) BG(%3d,%3d,%3d)\n",
					count, char, fg.R, fg.G, fg.B, bg.R, bg.G, bg.B)
			}
		}
		if count >= 12 {
			fmt.Println("     ... and 4,852 more combinations!")
			break
		}
	}
	
	fmt.Println()
	fmt.Println("Key Difference:")
	fmt.Println("- Current: Clever optimization that reduces search space")
	fmt.Println("- True: Brute force that guarantees finding the absolute best match")
	fmt.Println("- Trade-off: Speed vs optimality")
}