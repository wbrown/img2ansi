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
			"(Embedded: ansi16, ansi16bbs, ansi256, jetbrains32)")
	targetWidth := flag.Int("width", 80,
		"Target width of the output image")
	scaleFactor := flag.Float64("scale", 2.0,
		"Scale factor for the output image")
	maxChars := flag.Int("maxchars", 1048576,
		"Maximum number of characters in the output")
	_ = flag.Int("quantization", 256,
		"Quantization factor (deprecated in v1.0.0)")
	kdSearchDepth := flag.Int("kdsearch", 0,
		"KD-tree search depth (0=use fast precomputed tables, >0=runtime search)")
	threshold := flag.Float64("cache_threshold", 200.0,
		"Max error for approximate cache matches (higher=faster, lower=better quality)")
	colorMethod := flag.String("colormethod",
		"RGB", "Color distance method: RGB, LAB, or Redmean")
	bbsMode := flag.Bool("bbs", false,
		"Output BBS-compatible .ANS files (CP437 encoding, legacy escape codes, CR+LF)")
	iceColors := flag.Bool("ice", false,
		"Enable iCE colors for BBS mode: 16 background colors instead of 8.\n"+
			"    \tRequires -bbs. Viewers must support iCE (SyncTERM, PabloDraw)\n"+
			"    \tor bright backgrounds will blink instead")
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

	if *iceColors && !*bbsMode {
		fmt.Fprintln(os.Stderr, "Warning: -ice has no effect without -bbs")
	}

	// BBS mode determines the palette. Without iCE colors only 8
	// backgrounds are expressible, so the block search must be restricted
	// to the ansi16bbs palette (16 fg / 8 bg colors); with -ice all 16
	// backgrounds of ansi16 are available via the blink attribute.
	if *bbsMode {
		bbsPalette := "ansi16bbs"
		if *iceColors {
			bbsPalette = "ansi16"
		}
		if *paletteFile != "ansi16" && *paletteFile != bbsPalette {
			fmt.Fprintf(os.Stderr,
				"Warning: -bbs overrides -palette %s with %s\n",
				*paletteFile, bbsPalette)
		}
		*paletteFile = bbsPalette
	}

	// Build renderer options
	opts := []img2ansi.RendererOption{
		img2ansi.WithTargetWidth(*targetWidth),
		img2ansi.WithScaleFactor(*scaleFactor),
		img2ansi.WithMaxChars(*maxChars),
		img2ansi.WithKdSearch(*kdSearchDepth),
		img2ansi.WithCacheThreshold(*threshold),
		img2ansi.WithColorMethod(method),
	}
	if *bbsMode {
		opts = append(opts, img2ansi.WithBBSMode())
		if *iceColors {
			opts = append(opts, img2ansi.WithICEColors())
		}
	}
	// Palette must be loaded last (after color method and BBS mode are set)
	opts = append(opts, img2ansi.WithPalette(*paletteFile))

	// Create Renderer
	startInit := time.Now()
	r := img2ansi.NewRenderer(opts...)
	endInit := time.Now()

	// Error out if precomputed tables aren't available (would be too slow)
	if !r.UsingPrecomputedTables() {
		fmt.Fprintf(os.Stderr, "Error: No precomputed tables for colormethod %q.\n", *colorMethod)
		fmt.Fprintf(os.Stderr, "Use -colormethod with one of: RGB, LAB, Redmean\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Renderer initialized\n"+
		"colormethod: %s\n"+
		"Initialization time: %v\n",
		*colorMethod, endInit.Sub(startInit))

	// Generate the art; both modes produce raw bytes plus their own
	// size statistics. BBS output is CP437 (not valid UTF-8), standard
	// output is compressed UTF-8 ANSI.
	var art []byte
	var sizeStats string
	if *bbsMode {
		bbsArt, err := r.ImageToBBS(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error converting image: %v\n", err)
			os.Exit(1)
		}
		art = bbsArt
		sizeStats = fmt.Sprintf("BBS output size: %d bytes\n", len(art))
	} else {
		ansiArt, err := r.ImageToANSI(*inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error converting image: %v\n", err)
			os.Exit(1)
		}
		compressedArt := r.CompressANSI(ansiArt)
		art = []byte(compressedArt)
		sizeStats = fmt.Sprintf("Total string length: %d\nCompressed string length: %d\n",
			len(ansiArt), len(compressedArt))
	}
	endComputation := time.Now()

	// Output result
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, art, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Output written to %s\n", *outputFile)
	} else {
		os.Stdout.Write(art)
	}

	// Statistics go to stderr so art piped through stdout stays clean
	hits, misses, hitRate := r.CacheStats()
	uniqueKeys, sharedKeys, totalBlocks, avgError := r.CacheKeyStats()
	fmt.Fprintf(os.Stderr, "Computation time: %v\n", endComputation.Sub(endInit))
	fmt.Fprintf(os.Stderr, "BestBlock calculation time: %v\n", r.GetBestBlockTime())
	fmt.Fprint(os.Stderr, sizeStats)
	fmt.Fprintf(os.Stderr, "Block Cache: %d hits, %d misses (%.1f%% hit rate)\n",
		hits, misses, hitRate*100)
	fmt.Fprintf(os.Stderr, "Cache Keys: %d unique, %d shared (%d blocks, avg error %.1f)\n",
		uniqueKeys, sharedKeys, totalBlocks, avgError)
}
