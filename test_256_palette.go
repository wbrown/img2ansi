//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/wbrown/img2ansi"
)

func main() {
	fmt.Println("Testing 256-color palette loading...")

	start := time.Now()
	img2ansi.CurrentColorDistanceMethod = img2ansi.MethodLAB
	fg, bg, err := img2ansi.LoadPalette("colordata/ansi256.json")
	if err != nil {
		log.Fatalf("Failed to load palette: %v", err)
	}

	loadTime := time.Since(start)
	fmt.Printf("Palette loaded in %v\n", loadTime)
	fmt.Printf("Foreground colors: %d\n", len(fg))
	fmt.Printf("Background colors: %d\n", len(bg))

	// Check if color tables were computed
	if img2ansi.GetFgClosestColor() != nil {
		fmt.Printf("Foreground color table size: %d\n", len(*img2ansi.GetFgClosestColor()))
	}
	if img2ansi.GetBgClosestColor() != nil {
		fmt.Printf("Background color table size: %d\n", len(*img2ansi.GetBgClosestColor()))
	}
}
