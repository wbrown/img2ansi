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
	quantization := flag.Int("quantization", 256,
		"Quantization factor")
	kdSearchDepth := flag.Int("kdsearch", 50,
		"Number of nearest neighbors to search in KD-tree, 0 to disable")
	threshold := flag.Float64("cache_threshold", 40.0,
		"Threshold for block cache")
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

	// Update global variables
	img2ansi.TargetWidth = *targetWidth
	img2ansi.MaxChars = *maxChars
	img2ansi.Quantization = *quantization
	img2ansi.ScaleFactor = *scaleFactor
	img2ansi.KdSearch = *kdSearchDepth
	img2ansi.CacheThreshold = *threshold

	*colorMethod = strings.ToLower(*colorMethod)
	switch *colorMethod {
	case "rgb":
		img2ansi.CurrentColorDistanceMethod = img2ansi.MethodRGB
	case "lab":
		img2ansi.CurrentColorDistanceMethod = img2ansi.MethodLAB
	case "redmean":
		img2ansi.CurrentColorDistanceMethod = img2ansi.MethodRedmean
	default:
		fmt.Println("Invalid color distance method, options are RGB," +
			" LAB, or Redmean")
		os.Exit(1)
	}

	fg, bg, err := img2ansi.LoadPalette(*paletteFile)
	if err != nil {
		fmt.Printf("Error loading palette: %v\n", err)
		os.Exit(1)
	}
	endInit := time.Now()
	fmt.Printf(
		"fg, bg, distinct colors: %d, %d, %d\n"+
			"colormethod: %s\n"+
			"distance table entries precomputed: %d\n",
		len(*fg.ColorArr), len(*bg.ColorArr), img2ansi.DistinctColors,
		*colorMethod, len(*fg.ClosestColorArr)+len(*bg.ClosestColorArr))
	fmt.Printf("Initialization time: %v\n",
		endInit.Sub(img2ansi.BeginInitTime))

	if len(os.Args) < 2 {
		fmt.Println("Please provide the path to the image as an argument")
		return
	}

	// Generate ANSI art
	ansiArt := img2ansi.ImageToANSI(*inputFile)
	compressedArt := img2ansi.CompressANSI(ansiArt)
	//compressedArt := ansiArt
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

	fmt.Printf("Computation time: %v\n", endComputation.Sub(endInit))
	fmt.Printf("BestBlock calculation time: %v\n", img2ansi.BestBlockTime)
	fmt.Printf("Total string length: %d\n", len(ansiArt))
	fmt.Printf("Compressed string length: %d\n", len(compressedArt))
	fmt.Printf("Block Cache: %d hits, %d misses\n",
		img2ansi.LookupHits, img2ansi.LookupMisses)
}
