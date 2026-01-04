package main

import (
	"flag"
	"fmt"
	"github.com/wbrown/img2ansi"
	"os"
	"strings"
	"time"
)

// printAnsiTable prints a table of ANSI colors and their corresponding
// codes for both foreground and background colors. The table is printed
// to stdout.
//func printAnsiTable(fgAnsi, bgAnsi *[]img2ansi.RGB) {
//	// Header
//	fgColors := make([]uint32, 0, len()
//	fgAnsi.Iterate(func(key, value interface{}) {
//		fgColors = append(fgColors, key.(uint32))
//	})
//	bgColors := make([]uint32, 0, bgAnsi.Len())
//	bgAnsi.Iterate(func(key, value interface{}) {
//		bgColors = append(bgColors, key.(uint32))
//	})
//	fmt.Printf("%17s", " ")
//	for _, fg := range fgColors {
//		fgAns, _ := fgAnsi.Get(fg)
//		fmt.Printf(" %6x (%3s) ", fg, fgAns)
//	}
//	fmt.Println()
//	for _, bg := range bgColors {
//		bgAns, _ := bgAnsi.Get(bg)
//		fmt.Printf("   %6x (%3s) ", bg, bgAns)
//
//		for _, fg := range fgColors {
//			fgAns, _ := fgAnsi.Get(fg)
//			bgAns, _ := bgAnsi.Get(bg)
//			fmt.Printf("    %s[%s;%sm %3s %3s %s[0m ",
//				ESC, fgAns, bgAns, fgAns, bgAns, ESC)
//		}
//		fmt.Println()
//	}
//}

func main() {
	inputFile := flag.String("input", "",
		"Path to the input image file (required)")
	outputFile := flag.String("output", "",
		"Path to save the output (if not specified, prints to stdout)")
	paletteFile := flag.String("palette", "ansi16",
		"Path to the palette file "+
			"(Embedded: ansi16, ansi256, jetbrains32)")
	targetWidth := flag.Int("width", 80,
		"Target width of the output image")
	scaleFactor := flag.Float64("scale", 2.0,
		"Scale factor for the output image")
	maxChars := flag.Int("maxchars", 1048576,
		"Maximum number of characters in the output")
	_ = flag.Int("quantization", 256,
		"Quantization factor (deprecated in v1.0.0)")
	kdSearchDepth := flag.Int("kdsearch", 50,
		"Number of nearest neighbors to search in KD-tree, 0 to disable")
	threshold := flag.Float64("cache_threshold", 40.0,
		"Max error for approximate cache matches (higher=faster, lower=better quality)")
	colorMethod := flag.String("colormethod",
		"RGB", "Color distance method: RGB, LAB, or Redmean")
	//printTable := flag.Bool("table", false,
	//	"Print ANSI color table")
	// Parse flags
	flag.Parse()

	// Validate required flags
	if *inputFile == "" {
		fmt.Println("Please provide the image using the -input flag")
		flag.PrintDefaults()
		return
	}

	//if *printTable {
	//	printAnsiTable()
	//	return
	//}

	// Build Renderer options
	*colorMethod = strings.ToLower(*colorMethod)
	var method img2ansi.ColorDistanceMethod
	switch *colorMethod {
	case "rgb":
		method = img2ansi.RGBMethod{}
	case "lab":
		method = img2ansi.LABMethod{}
	case "redmean":
		method = img2ansi.RedmeanMethod{}
	default:
		fmt.Println("Invalid color distance method, options are RGB, LAB, or Redmean")
		os.Exit(1)
	}

	// Create Renderer
	startInit := time.Now()
	r := img2ansi.NewRenderer(
		img2ansi.WithTargetWidth(*targetWidth),
		img2ansi.WithScaleFactor(*scaleFactor),
		img2ansi.WithMaxChars(*maxChars),
		img2ansi.WithKdSearch(*kdSearchDepth),
		img2ansi.WithCacheThreshold(*threshold),
		img2ansi.WithColorMethod(method),
		img2ansi.WithPalette(*paletteFile),
	)
	endInit := time.Now()

	fmt.Printf("Renderer initialized\n"+
		"colormethod: %s\n"+
		"Initialization time: %v\n",
		*colorMethod, endInit.Sub(startInit))

	if len(os.Args) < 2 {
		fmt.Println("Please provide the path to the image as an argument")
		return
	}

	// Generate ANSI art
	ansiArt, err := r.ImageToANSI(*inputFile)
	if err != nil {
		fmt.Printf("Error converting image: %v\n", err)
		os.Exit(1)
	}
	compressedArt := r.CompressANSI(ansiArt)
	endComputation := time.Now()

	// Output result
	if *outputFile != "" {
		err := os.WriteFile(*outputFile, []byte(compressedArt), 0644)
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
			return
		}
		fmt.Printf("Output written to %s\n", *outputFile)
	} else {
		fmt.Print(compressedArt)
	}

	hits, misses, hitRate := r.CacheStats()
	uniqueKeys, sharedKeys, totalBlocks, avgError := r.CacheKeyStats()
	fmt.Printf("Computation time: %v\n", endComputation.Sub(endInit))
	fmt.Printf("BestBlock calculation time: %v\n", r.GetBestBlockTime())
	fmt.Printf("Total string length: %d\n", len(ansiArt))
	fmt.Printf("Compressed string length: %d\n", len(compressedArt))
	fmt.Printf("Block Cache: %d hits, %d misses (%.1f%% hit rate)\n",
		hits, misses, hitRate*100)
	fmt.Printf("Cache Keys: %d unique, %d shared (%d blocks, avg error %.1f)\n",
		uniqueKeys, sharedKeys, totalBlocks, avgError)
}
