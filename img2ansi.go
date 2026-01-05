package img2ansi

import (
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"time"

	"github.com/wbrown/img2ansi/imageutil"
)

const (
	ESC = "\u001b"
)

// Blocks defines the 16 Unicode block drawing characters used for 2x2 pixel blocks.
// The ordering is important: each index encodes which quadrants are filled as a 4-bit pattern.
var Blocks = []blockDef{
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

type blockDef struct {
	Rune rune
	Quad Quadrants
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
func (r *Renderer) BrownDitherForBlocks(
	img *imageutil.RGBAImage,
	edges *imageutil.GrayImage,
) [][]BlockRune {
	height, width := img.Height(), img.Width()
	blockHeight, blockWidth := height/2, width/2
	result := make([][]BlockRune, blockHeight)
	for i := range result {
		result[i] = make([]BlockRune, blockWidth)
	}

	for by := 0; by < blockHeight; by++ {
		for bx := 0; bx < blockWidth; bx++ {
			// Get the 2x2 block (note: imageutil uses x,y ordering)
			block := [4]RGB{
				rgbFromImageutil(img.GetRGB(bx*2, by*2)),
				rgbFromImageutil(img.GetRGB(bx*2+1, by*2)),
				rgbFromImageutil(img.GetRGB(bx*2, by*2+1)),
				rgbFromImageutil(img.GetRGB(bx*2+1, by*2+1)),
			}

			// Determine if this is an edge block
			isEdge := edges.GrayAt(bx*2, by*2).Y > 128 ||
				edges.GrayAt(bx*2+1, by*2).Y > 128 ||
				edges.GrayAt(bx*2, by*2+1).Y > 128 ||
				edges.GrayAt(bx*2+1, by*2+1).Y > 128

			// Find the best representation for this block
			bestRune, fgColor, bgColor := r.FindBestBlockRepresentation(
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
				var targetColor RGB
				// PERFORMANCE: This uses a bitwise operation on the rune value
				// instead of looking up quadrants. The Unicode block characters
				// are specifically chosen so their codepoints encode which
				// quadrants are filled. This is a critical hot path optimization.
				if (bestRune & (1 << (3 - i))) != 0 {
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

// FindBestBlockRepresentation finds the optimal (rune, fg, bg) for a 2x2 pixel block.
//
// The algorithm has two stages:
//
// Stage 1: Per-Pixel Color Mapping
// For each of the 4 pixels, find the closest palette color. This uses either:
//   - Precomputed tables (O(1) lookup) for built-in ColorDistanceMethods
//   - KD-tree nearest neighbor search for custom methods
// This gives us 4 "anchor" colors - the per-pixel optima.
//
// Stage 2: Block Search
// We're constrained to 2 colors (fg, bg) for all 4 pixels, so the per-pixel
// optima may not be achievable. We search for the best 2-color combination:
//   - Small palettes (≤32 colors): Brute force all fg×bg×pattern combinations
//   - Large palettes: KD-tree candidate search using anchors from Stage 1
//
// The KD-tree candidate search leverages the precomputed results: it finds
// colors NEAR each anchor, then tests combinations. This limits search from
// O(colors²) to O(depth²) while still finding high-quality solutions.
//
// Results are cached by the palette-mapped block key for reuse.
func (r *Renderer) FindBestBlockRepresentation(block [4]RGB, isEdge bool) (rune, RGB, RGB) {
	// Map each color in the block to its closest palette color
	var fgPaletteBlock [4]RGB
	var bgPaletteBlock [4]RGB

	// Use precomputed tables if available, otherwise use KD-tree lookup
	useKdTreeLookup := r.fgClosestColor == nil || r.bgClosestColor == nil
	for i, color := range block {
		if useKdTreeLookup {
			// Runtime KD-tree lookup for custom ColorDistanceMethod
			fgPaletteBlock[i], _ = r.fgTree.nearestNeighbor(
				color, r.fgTree.Color, math.MaxFloat64, 0, r.ColorMethod)
			bgPaletteBlock[i], _ = r.bgTree.nearestNeighbor(
				color, r.bgTree.Color, math.MaxFloat64, 0, r.ColorMethod)
		} else {
			// Fast table lookup for built-in methods
			fgPaletteBlock[i] = (*r.fgClosestColor)[color.toUint32()]
			bgPaletteBlock[i] = (*r.bgClosestColor)[color.toUint32()]
		}
	}
	blockKey := rgbsPairToUint256(fgPaletteBlock, bgPaletteBlock)

	// Check the block cache for a match
	if rune, fg, bg, found := r.getCacheEntry(
		blockKey, block, isEdge); found {
		return rune, fg, bg
	}
	startBlock := time.Now()

	// Use brute force search only for small palettes (<=32 colors)
	// For larger palettes, use KD-tree candidate search to avoid O(n²) explosion
	if r.distinctColors <= 32 {
		var bestRune rune
		var bestFG, bestBG RGB
		minError := math.MaxFloat64

		for _, b := range Blocks {
			r.fgAnsi.Iterate(func(fg, _ interface{}) {
				fgRgb := rgbFromUint32(fg.(uint32))
				r.bgAnsi.Iterate(func(bg, _ interface{}) {
					bgRgb := rgbFromUint32(bg.(uint32))
					if fg != bg {
						colorError := r.calculateBlockError(
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

		r.bestBlockTime += time.Since(startBlock)
		// Add the result to the lookup table
		r.addCacheEntry(blockKey, bestRune, bestFG, bestBG, block, isEdge)
		return bestRune, bestFG, bestBG
	}

	// Use KdSearch depth, or default to 50 if not specified (for large palettes with precomputed tables)
	searchDepth := r.KdSearch
	if searchDepth == 0 {
		searchDepth = 50
	}
	fgDepth := min(searchDepth, len(r.fgColors))
	bgDepth := min(searchDepth, len(r.bgColors))
	foregroundColors := r.fgTree.getCandidateColors(fgPaletteBlock, fgDepth, r.ColorMethod)
	backgroundColors := r.bgTree.getCandidateColors(bgPaletteBlock, bgDepth, r.ColorMethod)

	var bestRune rune
	var bestFG, bestBG RGB
	minError := math.MaxFloat64

	for _, b := range Blocks {
		for _, fgWithDist := range foregroundColors {
			for _, bgWithDist := range backgroundColors {
				fg, bg := fgWithDist.color, bgWithDist.color
				if fg == bg {
					continue
				}
				colorError := r.calculateBlockError(
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

	r.bestBlockTime += time.Since(startBlock)

	// Add the result to the lookup table
	r.addCacheEntry(blockKey, bestRune, bestFG, bestBG, block, isEdge)

	return bestRune, bestFG, bestBG
}

// calculateBlockError calculates the error between a 2x2 block of colors
// and a given representation of a block. The function takes the block of
// colors, the quadrants of the block representation, the foreground and
// background colors, and a boolean Value indicating whether the block is
// an edge block. It returns the error as a floating-point number.
func (r *Renderer) calculateBlockError(
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
		totalError += r.ColorMethod.Distance(color, targetColor)
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
	for _, b := range Blocks {
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
func distributeError(img *imageutil.RGBAImage, y, x int, error RGBError, isEdge bool) {
	height, width := img.Height(), img.Width()
	errorScale := 1.0
	if isEdge {
		errorScale = 0.5 // Reduce error diffusion for edge pixels
	}

	diffuseError := func(y, x int, factor float64) {
		if y >= 0 && y < height && x >= 0 && x < width {
			pixel := img.GetRGB(x, y) // Note: imageutil uses x,y ordering
			newR := uint8(math.Max(0, math.Min(255,
				float64(pixel.R)+float64(error.R)*factor*errorScale)))
			newG := uint8(math.Max(0, math.Min(255,
				float64(pixel.G)+float64(error.G)*factor*errorScale)))
			newB := uint8(math.Max(0, math.Min(255,
				float64(pixel.B)+float64(error.B)*factor*errorScale)))
			img.SetRGB(x, y, imageutil.RGB{R: newR, G: newG, B: newB})
		}
	}

	diffuseError(y, x+1, 7.0/16.0)
	diffuseError(y+1, x-1, 3.0/16.0)
	diffuseError(y+1, x, 5.0/16.0)
	diffuseError(y+1, x+1, 1.0/16.0)
}

// ImageToANSI converts an image to ANSI art. The function takes the path to
// an image file as a string and returns the image as an ANSI string.
func (r *Renderer) ImageToANSI(imagePath string) (string, error) {
	img, err := imageutil.LoadImage(imagePath)
	if err != nil {
		return "", fmt.Errorf("could not read image from %s: %v", imagePath, err)
	}

	aspectRatio := float64(img.Width()) / float64(img.Height())
	width := r.TargetWidth
	height := int(float64(width) / aspectRatio / r.ScaleFactor)

	for {
		resized, edges := imageutil.PrepareForANSI(img, width, height)
		ditheredImg := r.BrownDitherForBlocks(resized, edges)

		// Write the scaled image to a file for debugging
		if err := imageutil.SavePNG(resized.RGBA, "resized.png"); err != nil {
			fmt.Println(err)
		}

		// Write the dithered image to a file for debugging
		if err := saveBlocksToPNG(ditheredImg,
			"dithered.png",
			len(ditheredImg[0])*8,
			int(float64(len(ditheredImg)*8)*r.ScaleFactor),
			r.ScaleFactor,
		); err != nil {
			fmt.Println(err)
		}

		// Write the edges image to a file for debugging
		if err := imageutil.SaveGrayImage(edges, "edges.png"); err != nil {
			fmt.Println(err)
		}

		ansiImage := r.RenderToAnsi(ditheredImg)
		if len(ansiImage) <= r.MaxChars {
			return ansiImage, nil
		}

		width -= 2
		height = int(float64(width) / aspectRatio / 2)
		if width < 10 {
			return "", fmt.Errorf("image too large to fit within character limit")
		}
	}
}
