package img2ansi

import (
	"fmt"
	"gocv.io/x/gocv"
	_ "image/png"
	"math"
	"time"
)

const (
	ESC = "\u001b"
)

var (
	TargetWidth    = 100
	ScaleFactor    = 3.0
	MaxChars       = 1048576
	Quantization   = 1
	KdSearch       = 0
	CacheThreshold = 50.0

	blocks = []blockDef{
		{' ', Quadrants{false, false, false, false}}, // 0000: Empty space
		{'▗', Quadrants{false, false, false, true}},  // 0001: Quadrant lower right
		{'▖', Quadrants{false, false, true, false}},  // 0010: Quadrant lower left
		{'▄', Quadrants{false, false, true, true}},   // 0011: Lower half block
		{'▝', Quadrants{false, true, false, false}},  // 0100: Quadrant upper right
		{'▐', Quadrants{false, true, false, true}},   // 0101: Right half block
		{'▞', Quadrants{false, true, true, false}},   // 0110: Diagonal upper right and lower left
		{'▟', Quadrants{false, true, true, true}},    // 0111: Three quadrants: upper right, lower left, lower right
		{'▘', Quadrants{true, false, false, false}},  // 1000: Quadrant upper left
		{'▚', Quadrants{true, false, false, true}},   // 1001: Diagonal upper left and lower right
		{'▌', Quadrants{true, false, true, false}},   // 1010: Left half block
		{'▙', Quadrants{true, false, true, true}},    // 1011: Three quadrants: upper left, lower left, lower right
		{'▀', Quadrants{true, true, false, false}},   // 1100: Upper half block
		{'▜', Quadrants{true, true, false, true}},    // 1101: Three quadrants: upper left, upper right, lower right
		{'▛', Quadrants{true, true, true, false}},    // 1110: Three quadrants: upper left, upper right, lower left
		{'█', Quadrants{true, true, true, true}},     // 1111: Full block
	}

	fgAnsiRev = map[string]uint32{}
	bgAnsiRev = map[string]uint32{}

	fgClosestColor *[]RGB
	bgClosestColor *[]RGB

	lookupTable    ApproximateCache
	LookupHits     int
	LookupMisses   int
	BeginInitTime  time.Time
	BestBlockTime  time.Duration
	bgTree         *ColorNode
	fgTree         *ColorNode
	DistinctColors int
)

type blockDef struct {
	Rune rune
	Quad Quadrants
}

func init() {
	lookupTable = make(ApproximateCache)
	BeginInitTime = time.Now()
	LookupHits = 0
	LookupMisses = 0
	initLab()
}

// buildReverseMap builds the reverse map for the ANSI color codes, it is
// used to look up the ANSI color code for a given RGB color.
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
// Value, where true indicates that the quadrant should be colored with the
// foreground color, and false indicates that the quadrant should be colored
// with the background color.
type Quadrants struct {
	TopLeft     bool
	TopRight    bool
	BottomLeft  bool
	BottomRight bool
}

// BrownDitherForBlocks applies a modified Floyd-Steinberg dithering
// algorithm to an image operating on 2x2 blocks rather than pixels. The
// function takes an input image and a binary image with edges detected. It
// returns a BlockRune representation with the dithering algorithm applied,
// with colors quantized to the nearest ANSI color.
func BrownDitherForBlocks(
	img gocv.Mat,
	edges gocv.Mat,
) [][]BlockRune {
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
			bestRune, fgColor, bgColor := findBestBlockRepresentation(
				block, isEdge)

			// Store the result
			result[by][bx] = BlockRune{
				Rune: bestRune,
				FG:   fgColor,
				BG:   bgColor,
			}

			// Calculate and distribute the error
			for i, blockColor := range block {
				y, x := by*2+i/2, bx*2+i%2
				// Find the index of this rune in the blocks array
				var runeIndex int
				for idx, b := range blocks {
					if b.Rune == bestRune {
						runeIndex = idx
						break
					}
				}

				// The array indices encode quadrant information:
				// bit 3: top-left, bit 2: top-right, bit 1: bottom-left, bit 0: bottom-right
				var targetColor RGB
				if (runeIndex & (1 << (3 - i))) != 0 {
					targetColor = fgColor
				} else {
					targetColor = bgColor
				}

				colorError := blockColor.subtractToError(targetColor)
				distributeError(img, y, x, colorError, isEdge)
			}
		}
	}

	return result
}

// findBestBlockRepresentation finds the best rune representation for a 2x2
// block of colors. The function takes the block of colors, a boolean Value
// indicating whether the block is an edge block, and returns the best rune
// representation, the foreground color, and the background color for the
// block.
func findBestBlockRepresentation(block [4]RGB, isEdge bool) (rune, RGB, RGB) {
	// Map each color in the block to its closest palette color
	var fgPaletteBlock [4]RGB
	var bgPaletteBlock [4]RGB
	for i, color := range block {
		if fgClosestColor == nil || bgClosestColor == nil {
			panic("fgClosestColor or bgClosestColor is nil")
		}
		fgPaletteBlock[i] = (*fgClosestColor)[color.toUint32()]
		bgPaletteBlock[i] = (*bgClosestColor)[color.toUint32()]
	}
	blockKey := rgbsPairToUint256(fgPaletteBlock, bgPaletteBlock)

	// Check the block cache for a match
	if r, fg, bg, found := lookupTable.getEntry(
		blockKey, block, isEdge); found {
		return r, fg, bg
	}
	startBlock := time.Now()

	if KdSearch == 0 || DistinctColors < int(CacheThreshold) {
		var bestRune rune
		var bestFG, bestBG RGB
		minError := math.MaxFloat64

		for _, b := range blocks {
			fgAnsi.Iterate(func(fg, _ interface{}) {
				fgRgb := rgbFromUint32(fg.(uint32))
				bgAnsi.Iterate(func(bg, _ interface{}) {
					bgRgb := rgbFromUint32(bg.(uint32))
					if fg != bg {
						colorError := calculateBlockError(
							block, b.Quad, fgRgb, bgRgb, isEdge)
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

		BestBlockTime += time.Since(startBlock)
		// Add the result to the lookup table
		lookupTable.addEntry(blockKey, bestRune, bestFG, bestBG, block, isEdge)
		return bestRune, bestFG, bestBG
	}

	fgDepth := min(KdSearch, len(fgColors))
	bgDepth := min(KdSearch, len(bgColors))
	foregroundColors := fgTree.getCandidateColors(fgPaletteBlock, fgDepth)
	backgroundColors := bgTree.getCandidateColors(bgPaletteBlock, bgDepth)

	var bestRune rune
	var bestFG, bestBG RGB
	minError := math.MaxFloat64

	for _, b := range blocks {
		for _, fgWithDist := range foregroundColors {
			for _, bgWithDist := range backgroundColors {
				fg, bg := fgWithDist.color, bgWithDist.color
				if fg == bg {
					continue
				}
				colorError := calculateBlockError(
					block, b.Quad, fg, bg, isEdge)
				// Round error to reduce floating-point variability
				if colorError < minError ||
					(math.Abs(colorError-minError) < epsilon &&
						(fg.R < bestFG.R ||
							(fg.R == bestFG.R && fg.G < bestFG.G) ||
							(fg.R == bestFG.R &&
								fg.G == bestFG.G && fg.B < bestFG.B))) {
					minError = colorError
					bestRune = b.Rune
					bestFG = fg
					bestBG = bg
				}
			}
		}
	}

	BestBlockTime += time.Since(startBlock)

	// Add the result to the lookup table
	lookupTable.addEntry(blockKey, bestRune, bestFG, bestBG, block, isEdge)

	return bestRune, bestFG, bestBG
}

// calculateBlockError calculates the error between a 2x2 block of colors
// and a given representation of a block. The function takes the block of
// colors, the quadrants of the block representation, the foreground and
// background colors, and a boolean Value indicating whether the block is
// an edge block. It returns the error as a floating-point number.
func calculateBlockError(
	block [4]RGB,
	quad Quadrants,
	fg, bg RGB,
	isEdge bool,
) float64 {
	var totalError float64
	quadrants := [4]bool{
		quad.TopLeft, quad.TopRight,
		quad.BottomLeft, quad.BottomRight,
	}
	for i, color := range block {
		var targetColor RGB
		if quadrants[i] {
			targetColor = fg
		} else {
			targetColor = bg
		}
		totalError += color.ColorDistance(targetColor)
	}
	if isEdge {
		totalError *= 0.5
	}
	return totalError
}

// getQuadrantsForRune returns the quadrants for a given rune character.
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
// and a boolean Value indicating whether the pixel is an edge pixel.
func distributeError(img gocv.Mat, y, x int, error RGBError, isEdge bool) {
	height, width := img.Rows(), img.Cols()
	errorScale := 1.0
	if isEdge {
		errorScale = 0.5 // Reduce error diffusion for edge pixels
	}

	diffuseError := func(y, x int, factor float64) {
		if y >= 0 && y < height && x >= 0 && x < width {
			pixel := rgbFromVecb(img.GetVecbAt(y, x))
			newR := uint8(math.Max(0, math.Min(255,
				float64(pixel.R)+float64(error.R)*factor*errorScale)))
			newG := uint8(math.Max(0, math.Min(255,
				float64(pixel.G)+float64(error.G)*factor*errorScale)))
			newB := uint8(math.Max(0, math.Min(255,
				float64(pixel.B)+float64(error.B)*factor*errorScale)))
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

// ImageToBlocks converts an image to BlockRune array. The function takes the path to
// an image file as a string and returns the image as a 2D array of BlockRune.
// Also returns the final width and height used for processing.
func ImageToBlocks(imagePath string) ([][]BlockRune, int, int, error) {
	img := gocv.IMRead(imagePath, gocv.IMReadAnyColor)
	if img.Empty() {
		return nil, 0, 0, fmt.Errorf("could not read image from %s", imagePath)
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
		ditheredImg := BrownDitherForBlocks(resized, edges)
		// Write the scaled image to a file for debugging
		if err := saveToPNG(resized, "resized.png"); err != nil {
			fmt.Println(err)
		}

		// Write the dithered image to a file for debugging
		if err := saveBlocksToPNG(ditheredImg,
			"dithered.png",
			len(ditheredImg[0])*8,
			int(float64(len(ditheredImg)*8)*ScaleFactor),
			ScaleFactor,
		); err != nil {
			fmt.Println(err)
		}

		// Write the edges image to a file for debugging
		if err := saveToPNG(edges, "edges.png"); err != nil {
			fmt.Println(err)

		}

		ansiImage := RenderToAnsi(ditheredImg)
		if len(ansiImage) <= MaxChars {
			return ditheredImg, width, height, nil
		}

		width -= 2
		height = int(float64(width) / aspectRatio / 2)
		if width < 10 {
			return nil, 0, 0, fmt.Errorf("image too large to fit within character limit")
		}
	}
}

// ImageToANSI converts an image to ANSI art. The function takes the path to
// an image file as a string and returns the image as an ANSI string.
func ImageToANSI(imagePath string) string {
	blocks, _, _, err := ImageToBlocks(imagePath)
	if err != nil {
		return err.Error()
	}
	return RenderToAnsi(blocks)
}
