package main

import (
	"flag"
	"fmt"
	"gocv.io/x/gocv"
	_ "image/png"
	"math"
	"os"
	"sort"
	"time"
)

const (
	ESC = "\u001b"
)

var (
	TargetWidth  = 100
	ScaleFactor  = 3.0
	EdgeDistance = uint32(1250)
	MaxChars     = 1048576
	Quantization = 1
	kdSearch     = 40
	fgAnsiData   = []struct {
		key   uint32
		value string
	}{
		{0x000000, "30"}, {0xEB5156, "31"}, {0x69953D, "32"}, {0xA28B2F, "33"},
		{0x5291CF, "34"}, {0x9F73BA, "35"}, {0x48A0A2, "36"}, {0x808080, "37"},
		{0x4D4D4D, "90"}, {0xEF5357, "91"}, {0x70C13E, "92"}, {0xE3C23C, "93"},
		{0x54AFF9, "94"}, {0xDF84E7, "95"}, {0x67E0E1, "96"}, {0xC0C0C0, "97"},
	}
	fgAnsi        = NewOrderedMap()
	fgAnsi256Data = []struct {
		key   uint32
		value string
	}{
		{0x000000, "38;5;0"}, {0x800000, "38;5;1"}, {0x008000, "38;5;2"},
		{0x808000, "38;5;3"}, {0x000080, "38;5;4"}, {0x800080, "38;5;5"},
		{0x008080, "38;5;6"}, {0xC0C0C0, "38;5;7"}, {0x808080, "38;5;8"},
		{0xFF0000, "38;5;9"}, {0x00FF00, "38;5;10"}, {0xFFFF00, "38;5;11"},
		{0x0000FF, "38;5;12"}, {0xFF00FF, "38;5;13"}, {0x00FFFF, "38;5;14"},
		{0xFFFFFF, "38;5;15"},
	}
	fgAnsi256     = NewOrderedMap()
	bgAnsi        = NewOrderedMap()
	bgAnsi256     = NewOrderedMap()
	ansiOverrides = map[uint32]string{}

	blocks = []blockDef{
		{' ', Quadrants{false, false, false, false}}, // Empty space
		{'▘', Quadrants{true, false, false, false}},  // Quadrant upper left
		{'▝', Quadrants{false, true, false, false}},  // Quadrant upper right
		{'▀', Quadrants{true, true, false, false}},   // Upper half block
		{'▖', Quadrants{false, false, true, false}},  // Quadrant lower left
		{'▌', Quadrants{true, false, true, false}},   // Left half block
		{'▞', Quadrants{false, true, true, false}},   // Quadrant diagonal upper right and lower left
		{'▛', Quadrants{true, true, true, false}},    // Three quadrants: upper left, upper right, lower left
		{'▗', Quadrants{false, false, false, true}},  // Quadrant lower right
		{'▚', Quadrants{true, false, false, true}},   // Quadrant diagonal upper left and lower right
		{'▐', Quadrants{false, true, false, true}},   // Right half block
		{'▜', Quadrants{true, true, false, true}},    // Three quadrants: upper left, upper right, lower right
		{'▄', Quadrants{false, false, true, true}},   // Lower half block
		{'▙', Quadrants{true, false, true, true}},    // Three quadrants: upper left, lower left, lower right
		{'▟', Quadrants{false, true, true, true}},    // Three quadrants: upper right, lower left, lower right
		{'█', Quadrants{true, true, true, true}},     // Full block
	}

	fgAnsiRev = map[string]uint32{}
	bgAnsiRev = map[string]uint32{}

	closestColor   = make([]RGB, 256*256*256)
	colors         = make([]RGB, 0)
	rgbColorTable  = make(map[RGB]uint32)
	colorDistances = make(map[RGB]map[RGB]float64)
	lookupTable    map[[4]RGB]lookupEntry
	lookupHits     int
	lookupMisses   int
	distanceHits   int
	distanceMisses int
	beginInitTime  time.Time
	kdTree         *ColorNode
)

type lookupEntry struct {
	rune rune
	fg   RGB
	bg   RGB
}

type blockDef struct {
	Rune rune
	Quad Quadrants
}

func init() {
	beginInitTime = time.Now()
	lookupHits = 0
	lookupMisses = 0
	distanceHits = 0
	distanceMisses = 0
	for _, data := range fgAnsiData {
		fgAnsi.Set(data.key, data.value)
	}
	for _, data := range fgAnsi256Data {
		fgAnsi256.Set(data.key, data.value)
	}

	// Generate 216 colors (6x6x6 color cube)
	colorCode := 16
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				red := uint32(r * 51)
				green := uint32(g * 51)
				blue := uint32(b * 51)
				color := (red << 16) | (green << 8) | blue
				fgAnsi256.Set(color, fmt.Sprintf("38;5;%d", colorCode))
				colorCode++
			}
		}
	}

	// Grayscale colors (232-255)
	for i := 0; i < 24; i++ {
		gray := uint32(i*10 + 8)
		color := (gray << 16) | (gray << 8) | gray
		fgAnsi256.Set(color, fmt.Sprintf("38;5;%d", colorCode))
		colorCode++
	}

	// Generate background 256 colors
	fgAnsi256.Iterate(func(key, value interface{}) {
		fgColor := key.(uint32)
		fgCode := value.(string)
		bgAnsi256.Set(fgColor, "4"+fgCode[1:])
	})

	for overrideColor, code := range ansiOverrides {
		fgAnsi.Iterate(func(key, value interface{}) {
			fgColor := key.(uint32)
			fgCode := value.(string)
			if fgCode == code {
				fgAnsi.Delete(fgColor)
				fgAnsi.Set(overrideColor, code)
			}
		})
	}

	// If bgaAnsi is empty, populate it
	if bgAnsi.Len() == 0 {
		fgAnsi.Iterate(func(key, value interface{}) {
			fgColor := key.(uint32)
			fgCode := value.(string)
			if fgCode[0] == '3' {
				bgAnsi.Set(fgColor, "4"+fgCode[1:])
			} else if fgCode[0] == '9' {
				bgAnsi.Set(fgColor, "10"+fgCode[1:])
			}
		})
	}

	buildReverseMap()
	computeColorDistances()
	lookupTable = make(map[[4]RGB]lookupEntry)
}

func buildReverseMap() {
	newFgAnsiRev := make(map[string]uint32)
	fgAnsi.Iterate(func(key, value interface{}) {
		fgColor := key.(uint32)
		fgCode := value.(string)
		newFgAnsiRev[fgCode] = fgColor
	})

	newBgAnsiRev := make(map[string]uint32)
	bgAnsi.Iterate(func(key, value interface{}) {
		bgColor := key.(uint32)
		bgCode := value.(string)
		newBgAnsiRev[bgCode] = bgColor
	})
	fgAnsiRev = newFgAnsiRev
	bgAnsiRev = newBgAnsiRev
}

func computeColorDistances() {
	colors = make([]RGB, 0, fgAnsi.Len())
	idx := uint32(0)
	fgAnsi.Iterate(func(key, value interface{}) {
		colors = append(colors, rgbFromUint32(key.(uint32)))
		rgbColorTable[rgbFromUint32(key.(uint32))] = idx
		idx++
	})

	maxDepth := int(math.Log2(float64(len(colors)))) + 1
	kdTree = buildKDTree(colors, 0, maxDepth)
	for r := 0; r < 256; r++ {
		for g := 0; g < 256; g++ {
			for b := 0; b < 256; b++ {
				rgb := RGB{uint8(r), uint8(g), uint8(b)}
				closest, _ := nearestNeighbor(
					kdTree, rgb, kdTree.Color, math.MaxFloat64, 0)
				closestColor[r<<16|g<<8|b] = closest
			}
		}
	}
}

// BlockRune represents a 2x2 block of runes with foreground and
// background colors mapped in the ANSI color space. The struct contains
// a rune representing the block character, and two RGB colors representing
// the foreground and background colors of the block.
type BlockRune struct {
	Rune rune
	FG   RGB
	BG   RGB
}

// Quadrants represents the four quadrants of a 2x2 block of a rune that
// can be colored independently. Each quadrant is represented by a boolean
// value, where true indicates that the quadrant should be colored with the
// foreground color, and false indicates that the quadrant should be colored
// with the background color.
type Quadrants struct {
	TopLeft     bool
	TopRight    bool
	BottomLeft  bool
	BottomRight bool
}

// modifiedAtkinsonDitherForBlocks applies the modified Atkinson dithering
// algorithm to an image operating on 2x2 blocks rather than pixels. The
// function takes an input image and a binary image with edges detected. It
// returns a BlockRune representation with the Atkinson dithering algorithm
// applied, with colors quantized to the nearest ANSI color.
func modifiedAtkinsonDitherForBlocks(img gocv.Mat, edges gocv.Mat) [][]BlockRune {
	height, width := img.Rows(), img.Cols()
	blockHeight, blockWidth := height/2, width/2
	result := make([][]BlockRune, blockHeight)
	for i := range result {
		result[i] = make([]BlockRune, blockWidth)
	}

	for by := 0; by < blockHeight; by++ {
		for bx := 0; bx < blockWidth; bx++ {
			// Get the 2x2 block
			block := [4]RGB{
				rgbFromVecb(img.GetVecbAt(by*2, bx*2)),
				rgbFromVecb(img.GetVecbAt(by*2, bx*2+1)),
				rgbFromVecb(img.GetVecbAt(by*2+1, bx*2)),
				rgbFromVecb(img.GetVecbAt(by*2+1, bx*2+1)),
			}

			// Determine if this is an edge block
			isEdge := edges.GetUCharAt(by*2, bx*2) > 128 ||
				edges.GetUCharAt(by*2, bx*2+1) > 128 ||
				edges.GetUCharAt(by*2+1, bx*2) > 128 ||
				edges.GetUCharAt(by*2+1, bx*2+1) > 128

			// Find the best representation for this block
			bestRune, fgColor, bgColor := findBestBlockRepresentation(block, isEdge)

			// Store the result
			result[by][bx] = BlockRune{
				Rune: bestRune,
				FG:   fgColor,
				BG:   bgColor,
			}

			// Calculate and distribute the error
			for i, blockColor := range block {
				y, x := by*2+i/2, bx*2+i%2
				var targetColor RGB
				if (bestRune & (1 << (3 - i))) != 0 {
					targetColor = fgColor
				} else {
					targetColor = bgColor
				}
				colorError := blockColor.subtract(targetColor)
				distributeError(img, y, x, colorError, isEdge)
			}
		}
	}

	return result
}

// findBestBlockRepresentation finds the best rune representation for a 2x2
// block of colors. The function takes the block of colors, a boolean value
// indicating whether the block is an edge block, and returns the best rune
// representation, the foreground color, and the background color for the
// block.
func findBestBlockRepresentation(block [4]RGB, isEdge bool) (rune, RGB, RGB) {
	// Map each color in the block to its closest palette color
	var paletteBlock [4]RGB
	for i, color := range block {
		paletteBlock[i] = closestColor[color.toUint32()]
	}

	// Check if the palette-mapped block is in the lookup table
	if entry, exists := lookupTable[paletteBlock]; exists {
		lookupHits++
		return entry.rune, entry.fg, entry.bg
	}
	lookupMisses++

	if fgAnsi.Len() < 32 || kdSearch == 0 {
		var bestRune rune
		var bestFG, bestBG RGB
		minError := math.MaxFloat64
		for _, b := range blocks {
			fgAnsi.Iterate(func(fg, _ interface{}) {
				fgRgb := rgbFromUint32(fg.(uint32))
				bgAnsi.Iterate(func(bg, _ interface{}) {
					bgRgb := rgbFromUint32(bg.(uint32))
					if fg != bg {
						colorError := calculateBlockError(block, b.Quad, fgRgb, bgRgb, isEdge)
						if colorError < minError {
							minError = colorError
							bestRune = b.Rune
							bestFG = fgRgb
							bestBG = bgRgb
						}
					}
				})
			})
		}
		return bestRune, bestFG, bestBG
	}

	// Find initial candidates using KD-tree and sort by distance
	var candidateColors colorDistanceSlice
	seenColors := make(map[RGB]bool)

	// Process block colors in a consistent order
	sortedBlockColors := make(sortableRGB, len(block))
	for i, c := range block {
		sortedBlockColors[i] = c
	}
	sort.Sort(sortedBlockColors)

	searchDepth := min(kdSearch, len(colors))

	for _, color := range sortedBlockColors {
		nearest := kNearestNeighbors(kdTree, color, searchDepth)
		for _, c := range nearest {
			if _, seen := seenColors[c]; !seen {
				distance := color.colorDistance(c)
				candidateColors = append(candidateColors, colorWithDistance{c, distance, len(candidateColors)})
				seenColors[c] = true
			}
		}
	}

	sort.Sort(candidateColors)

	var bestRune rune
	var bestFG, bestBG RGB
	minError := math.MaxFloat64

	for _, b := range blocks {
		for i, fgWithDist := range candidateColors {
			for j, bgWithDist := range candidateColors {
				if i == j { // Skip when fg and bg are the same
					continue
				}
				fg, bg := fgWithDist.color, bgWithDist.color
				colorError := calculateBlockError(block, b.Quad, fg, bg, isEdge)
				// Round error to reduce floating-point variability
				if colorError < minError || (math.Abs(colorError-minError) < epsilon &&
					(fg.r < bestFG.r || (fg.r == bestFG.r && fg.g < bestFG.g) ||
						(fg.r == bestFG.r && fg.g == bestFG.g && fg.b < bestFG.b))) {
					minError = colorError
					bestRune = b.Rune
					bestFG = fg
					bestBG = bg
				}
			}
		}
	}

	// Add the result to the lookup table
	lookupTable[paletteBlock] = lookupEntry{
		rune: bestRune,
		fg:   bestFG,
		bg:   bestBG,
	}

	return bestRune, bestFG, bestBG
}

// calculateBlockError calculates the error between a 2x2 block of colors
// and a given representation of a block. The function takes the block of
// colors, the quadrants of the block representation, the foreground and
// background colors, and a boolean value indicating whether the block is
// an edge block. It returns the error as a floating-point number.
func calculateBlockError(block [4]RGB, quad Quadrants, fg, bg RGB, isEdge bool) float64 {
	var totalError float64
	quadrants := [4]bool{quad.TopLeft, quad.TopRight, quad.BottomLeft, quad.BottomRight}
	for i, color := range block {
		var targetColor RGB
		if quadrants[i] {
			targetColor = fg
		} else {
			targetColor = bg
		}
		l, lOk := colorDistances[color]
		if !lOk {
			distanceMisses++
			colorDistances[color] = make(map[RGB]float64)
			l = colorDistances[color]
		}
		r, rOk := l[targetColor]
		if !rOk {
			r = color.colorDistance(targetColor)
			colorDistances[color][targetColor] = r
		}
		if lOk && rOk {
			distanceHits++
		} else {
			distanceMisses++
		}
		totalError += r
	}
	if isEdge {
		totalError *= 0.5
	}

	return totalError
}

// getQuadrantsForRune returns the quadrants for a given rune character.
// The function takes a rune character and returns the quadrants for the
// corresponding block character, or an empty Quadrants struct if the
// character is not found.
func getQuadrantsForRune(char rune) Quadrants {
	for _, b := range blocks {
		if b.Rune == char {
			return b.Quad
		}
	}
	// Return empty quadrants if character not found
	return Quadrants{}
}

// distributeError distributes the error from a pixel to its neighbors
// using the Floyd-Steinberg error diffusion algorithm. The function takes
// an image, the y and x coordinates of the pixel, the error to distribute,
// and a boolean value indicating whether the pixel is an edge pixel.
func distributeError(img gocv.Mat, y, x int, error RGB, isEdge bool) {
	height, width := img.Rows(), img.Cols()
	errorScale := 1.0
	if isEdge {
		errorScale = 0.5 // Reduce error diffusion for edge pixels
	}

	diffuseError := func(y, x int, factor float64) {
		if y >= 0 && y < height && x >= 0 && x < width {
			pixel := rgbFromVecb(img.GetVecbAt(y, x))
			newR := uint8(math.Max(0, math.Min(255, float64(pixel.r)+float64(error.r)*factor*errorScale)))
			newG := uint8(math.Max(0, math.Min(255, float64(pixel.g)+float64(error.g)*factor*errorScale)))
			newB := uint8(math.Max(0, math.Min(255, float64(pixel.b)+float64(error.b)*factor*errorScale)))
			img.SetUCharAt(y, x*3+2, newR)
			img.SetUCharAt(y, x*3+1, newG)
			img.SetUCharAt(y, x*3, newB)
		}
	}

	diffuseError(y, x+1, 7.0/16.0)
	diffuseError(y+1, x-1, 3.0/16.0)
	diffuseError(y+1, x, 5.0/16.0)
	diffuseError(y+1, x+1, 1.0/16.0)
}

// imageToANSI converts an image to ANSI art. The function takes the path to
// an image file as a string and returns the image as an ANSI string.
func imageToANSI(imagePath string) string {
	img := gocv.IMRead(imagePath, gocv.IMReadAnyColor)
	if img.Empty() {
		return fmt.Sprintf("Could not read image from %s", imagePath)
	}
	defer func(img *gocv.Mat) {
		err := img.Close()
		if err != nil {
			fmt.Println("Error closing image")
		}
	}(&img)

	aspectRatio := float64(img.Cols()) / float64(img.Rows())
	width := TargetWidth
	height := int(float64(width) / aspectRatio / ScaleFactor)

	for {
		resized, edges := prepareForANSI(img, width, height)
		ditheredImg := modifiedAtkinsonDitherForBlocks(resized, edges)
		// Write the scaled image to a file for debugging
		if err := saveToPNG(resized, "resized.png"); err != nil {
			fmt.Println(err)
		}

		// Write the dithered image to a file for debugging
		if err := saveBlocksToPNG(ditheredImg, "dithered.png"); err != nil {
			fmt.Println(err)
		}
		// Write the edges image to a file for debugging
		if err := saveToPNG(edges, "edges.png"); err != nil {
			fmt.Println(err)

		}

		ansiImage := renderToAnsi(ditheredImg)
		if len(ansiImage) <= MaxChars {
			return ansiImage
		}

		width -= 2
		height = int(float64(width) / aspectRatio / 2)
		if width < 10 {
			return "Image too large to fit within character limit"
		}
	}
}


// printAnsiTable prints a table of ANSI colors and their corresponding
// codes for both foreground and background colors. The table is printed
// to stdout.
func printAnsiTable() {
	// Header
	fgColors := make([]uint32, 0, fgAnsi.Len())
	fgAnsi.Iterate(func(key, value interface{}) {
		fgColors = append(fgColors, key.(uint32))
	})
	bgColors := make([]uint32, 0, bgAnsi.Len())
	bgAnsi.Iterate(func(key, value interface{}) {
		bgColors = append(bgColors, key.(uint32))
	})
	fmt.Printf("%17s", " ")
	for _, fg := range fgColors {
		fgAns, _ := fgAnsi.Get(fg)
		fmt.Printf(" %6x (%3s) ", fg, fgAns)
	}
	fmt.Println()
	for _, bg := range bgColors {
		bgAns, _ := bgAnsi.Get(bg)
		fmt.Printf("   %6x (%3s) ", bg, bgAns)

		for _, fg := range fgColors {
			fgAns, _ := fgAnsi.Get(fg)
			bgAns, _ := bgAnsi.Get(bg)
			fmt.Printf("    %s[%s;%sm %3s %3s %s[0m ",
				ESC, fgAns, bgAns, fgAns, bgAns, ESC)
		}
		fmt.Println()
	}
}

func main() {
	inputFile := flag.String("input", "", "Path to the input image file (required)")
	targetWidth := flag.Int("width", 100, "Target width of the output image")
	maxChars := flag.Int("maxchars", 1048576, "Maximum number of characters in the output")
	outputFile := flag.String("output", "", "Path to save the output (if not specified, prints to stdout)")
	quantization := flag.Int("quantization", 256, "Quantization factor")
	scaleFactor := flag.Float64("scale", 3.0, "Scale factor for the output image")
	eightBit := flag.Bool("8bit", false, "Use 8-bit ANSI colors (256 colors)")
	printTable := flag.Bool("table", false, "Print ANSI color table")
	kdSearchDepth := flag.Int("kdsearch", 40, "Number of nearest neighbors to search in KD-tree, 0 to disable")

	// Parse flags
	flag.Parse()

	// Validate required flags
	if *inputFile == "" {
		fmt.Println("Please provide the image using the -input flag")
		flag.PrintDefaults()
		return
	}

	if *printTable {
		printAnsiTable()
		return
	}

	// Update global variables
	TargetWidth = *targetWidth
	MaxChars = *maxChars
	Quantization = *quantization
	ScaleFactor = *scaleFactor
	kdSearch = *kdSearchDepth
	if *eightBit {
		fgAnsi = fgAnsi256
		bgAnsi = bgAnsi256
		buildReverseMap()
		computeColorDistances()
	}
	endInit := time.Now()
	fmt.Printf("Initialization time: %v\n", endInit.Sub(beginInitTime))
	fmt.Printf("%d colors in palette\n", len(colors))

	if len(os.Args) < 2 {
		fmt.Println("Please provide the path to the image as an argument")
		return
	}

	// Generate ANSI art
	ansiArt := imageToANSI(*inputFile)
	compressedArt := compressANSI(ansiArt)
	//compressedArt := ansiArt

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

	fmt.Printf("Total string length: %d\n", len(ansiArt))
	fmt.Printf("Compressed string length: %d\n", len(compressedArt))
	fmt.Printf("Cache: %d hits, %d misses, %d entries\n", lookupHits, lookupMisses, len(lookupTable))
	fmt.Printf("Distance: %d hits, %d misses, %d entries\n", distanceHits, distanceMisses, len(colorDistances))
	//printAnsiTable()
}
