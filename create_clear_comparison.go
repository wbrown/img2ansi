// +build ignore

package main

import (
	"gocv.io/x/gocv"
	"log"
	
	"github.com/wbrown/img2ansi"
)

func main() {
	// Load palette
	img2ansi.CurrentColorDistanceMethod = img2ansi.MethodLAB
	_, _, err := img2ansi.LoadPalette("colordata/ansi16.json")
	if err != nil {
		log.Fatalf("Failed to load palette: %v", err)
	}

	// Create a simple image that will clearly show the bug
	// Top half: white, Bottom half: black
	// This should use the upper half block character (▀)
	img := gocv.NewMatWithSize(8, 8, gocv.MatTypeCV8UC3)
	defer img.Close()

	// Fill top 4 rows with white
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.SetUCharAt(y, x*3, 255)   // B
			img.SetUCharAt(y, x*3+1, 255) // G
			img.SetUCharAt(y, x*3+2, 255) // R
		}
	}

	// Fill bottom 4 rows with black
	for y := 4; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetUCharAt(y, x*3, 0)   // B
			img.SetUCharAt(y, x*3+1, 0) // G
			img.SetUCharAt(y, x*3+2, 0) // R
		}
	}

	edges := gocv.NewMatWithSize(8, 8, gocv.MatTypeCV8U)
	defer edges.Close()

	// Process the image
	blocks := img2ansi.BrownDitherForBlocks(img, edges)
	
	// This should produce a 4x4 grid of upper half blocks
	log.Println("Expected: All blocks should be upper half (▀) with white FG, black BG")
	log.Println("Actual blocks:")
	for y := 0; y < len(blocks); y++ {
		for x := 0; x < len(blocks[0]); x++ {
			b := blocks[y][x]
			log.Printf("  [%d,%d]: Rune='%c' FG=RGB(%d,%d,%d) BG=RGB(%d,%d,%d)",
				x, y, b.Rune, b.FG.R, b.FG.G, b.FG.B, b.BG.R, b.BG.G, b.BG.B)
		}
	}

	// Save the output
	err = img2ansi.SaveBlocksToPNGWithOptions(blocks, "simple_upper_half_test.png", img2ansi.RenderOptions{
		UseFont: false,
		Scale:   32,
	})
	if err != nil {
		log.Fatalf("Failed to save: %v", err)
	}
	
	log.Println("\nCreated simple_upper_half_test.png")
	log.Println("This should show white on top, black on bottom")
}