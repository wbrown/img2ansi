package img2ansi

import (
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/wbrown/img2ansi/imageutil"
)

// drawBlock draws a 2x2 block of a rune with the given foreground and
// background colors at the specified position in an image. The function
// takes a pointer to an image, the x and y coordinates of the block, and
// the block character to draw.
func drawBlock(img *imageutil.RGBAImage, x, y int, block BlockRune) {
	quad := getQuadrantsForRune(block.Rune)
	quadrants := [4]bool{
		quad.TopLeft,
		quad.TopRight,
		quad.BottomLeft,
		quad.BottomRight}

	for i := 0; i < 4; i++ {
		dx, dy := i%2, i/2
		var r, g, b uint8
		if quadrants[i] {
			r, g, b = block.FG.R, block.FG.G, block.FG.B
		} else {
			r, g, b = block.BG.R, block.BG.G, block.BG.B
		}
		img.SetRGB(x+dx, y+dy, imageutil.RGB{R: r, G: g, B: b})
	}
}

// saveBlocksToPNG saves a 2D array of BlockRune structs to a PNG file.
// The function takes a 2D array of BlockRune structs and a filename as
// strings, and returns an error if the file cannot be saved.
func saveBlocksToPNG(
	blocks [][]BlockRune,
	filename string,
	targetWidth,
	targetHeight int,
	scaleFactor float64,
) error {
	blockHeight, blockWidth := len(blocks), len(blocks[0])

	var outputWidth, outputHeight int
	if targetWidth == 0 && targetHeight == 0 {
		// Unscaled mode: each block is 2x2 pixels
		outputWidth = blockWidth * 2
		outputHeight = blockHeight * 2
	} else {
		// Scaled mode
		outputWidth = targetWidth
		if outputWidth == 0 {
			outputWidth = blockWidth * 2
		}
		outputHeight = targetHeight
		if outputHeight == 0 {
			outputHeight = int(float64(blockHeight) * 2 * scaleFactor)
		}
	}

	// Create the output image using standard library
	rgbaImg := image.NewRGBA(image.Rect(0, 0, outputWidth, outputHeight))

	scaleX := float64(outputWidth) / float64(blockWidth*2)
	scaleY := float64(outputHeight) / float64(blockHeight*2)

	for y := 0; y < outputHeight; y++ {
		for x := 0; x < outputWidth; x++ {
			blockX := int(float64(x) / scaleX / 2)
			blockY := int(float64(y) / scaleY / 2)

			if blockX >= blockWidth {
				blockX = blockWidth - 1
			}
			if blockY >= blockHeight {
				blockY = blockHeight - 1
			}

			block := blocks[blockY][blockX]
			quad := getQuadrantsForRune(block.Rune)

			quadX := int(float64(x)/scaleX) % 2
			quadY := int(float64(y)/scaleY) % 2

			var r, g, b uint8
			if isQuadrantActive(quad, quadX, quadY) {
				r, g, b = block.FG.R, block.FG.G, block.FG.B
			} else {
				r, g, b = block.BG.R, block.BG.G, block.BG.B
			}

			rgbaImg.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	// Save the image
	f, createErr := os.Create(filename)
	if createErr != nil {
		return createErr
	}
	defer f.Close()

	return png.Encode(f, rgbaImg)
}

// isQuadrantActive returns true if the specified quadrant is active in the
// given block rune, and false otherwise.
func isQuadrantActive(quad Quadrants, x, y int) bool {
	switch {
	case x == 0 && y == 0:
		return quad.TopLeft
	case x == 1 && y == 0:
		return quad.TopRight
	case x == 0 && y == 1:
		return quad.BottomLeft
	case x == 1 && y == 1:
		return quad.BottomRight
	}
	return false
}

// drawScaledBlock draws a scaled 2x2 block to an image.
func drawScaledBlock(img *imageutil.RGBAImage, x, y int, block BlockRune, scale int) {
	quad := getQuadrantsForRune(block.Rune)
	quadrants := [4]bool{
		quad.TopLeft,
		quad.TopRight,
		quad.BottomLeft,
		quad.BottomRight}

	halfScale := scale / 2

	for qy := 0; qy < 2; qy++ {
		for qx := 0; qx < 2; qx++ {
			var r, g, b uint8
			if quadrants[qy*2+qx] {
				r, g, b = block.FG.R, block.FG.G, block.FG.B
			} else {
				r, g, b = block.BG.R, block.BG.G, block.BG.B
			}

			// Fill the quadrant with the color
			for dy := 0; dy < halfScale; dy++ {
				for dx := 0; dx < halfScale; dx++ {
					px := x + qx*halfScale + dx
					py := y + qy*halfScale + dy
					img.SetRGB(px, py, imageutil.RGB{R: r, G: g, B: b})
				}
			}
		}
	}
}
