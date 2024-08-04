package main

import (
	"flag"
	"fmt"
	"gocv.io/x/gocv"
	"image"
	"image/color"
	"image/png"
	_ "image/png"
	"math"
	"os"
	"strings"
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
	fgAnsi       = map[uint32]string{
		//	// Original colors (more likely to be selected)
		0x000000: "30", // BLACK
		0xEB5156: "31", // RED
		0x69953D: "32", // GREEN
		0xA28B2F: "33", // YELLOW
		0x5291CF: "34", // BLUE
		0x9F73BA: "35", // MAGENTA
		0x48A0A2: "36", // CYAN
		0x808080: "37", // WHITE
		// Additional colors (less likely to be selected)
		0x4D4D4D: "90", // BRIGHT BLACK (dark gray)
		0xEF5357: "91", // BRIGHT RED (darker)
		0x70C13E: "92", // BRIGHT GREEN (darker)
		0xE3C23C: "93", // BRIGHT YELLOW (darker)
		0x54AFF9: "94", // BRIGHT BLUE (darker)
		0xDF84E7: "95", // BRIGHT MAGENTA (darker)
		0x67E0E1: "96", // BRIGHT CYAN (darker)
		0xC0C0C0: "97", // BRIGHT WHITE (light gray)
	}
	// Below are measured from the terminal using a color picker
	ansiOverrides = map[uint32]string{}
	//ansiOverrides = map[uint32]string{
	//	0x000000: "30", // BLACK
	//	0xB93018: "31", // RED
	//	0x52BF37: "32", // GREEN
	//	0xFFFC7F: "33", // YELLOW
	//	0x0D23BF: "34", // BLUE
	//	0xBA3FC0: "35", // MAGENTA
	//	0x53C2C5: "36", // CYAN
	//	0xC8C7C7: "37", // WHITE
	//	0x686868: "90", // BRIGHT BLACK (dark gray)
	//	0xF1776D: "91", // BRIGHT RED (darker)
	//	0x8DF67A: "92", // BRIGHT GREEN (darker)
	//	0xFEFC7F: "93", // BRIGHT YELLOW (darker)
	//	0x6A71F6: "94", // BRIGHT BLUE (darker)
	//	0xF07FF8: "95", // BRIGHT MAGENTA (darker)
	//	0x8EFAFD: "96", // BRIGHT CYAN (darker)
	//	0xFFFFFF: "97", // BRIGHT WHITE (light gray)
	//}

	// Original colors
	//0x000000: "30", // BLACK
	//0xF0524F: "31", // RED
	//0x5C962C: "32", // GREEN
	//0xA68A0D: "33", // YELLOW
	//0x3993D4: "34", // BLUE
	//0xA771BF: "35", // MAGENTA
	//0x00A3A3: "36", // CYAN
	//0x808080: "37", // WHITE

	// Bright colors
	//0x575959: "90", // BRIGHT BLACK (dark gray)
	//0xFF4050: "91", // BRIGHT RED (darker)
	//0x4FC414: "92", // BRIGHT GREEN (darker)
	//0xE5BF00: "93", // BRIGHT YELLOW (darker)
	//0x1FB0FF: "94", // BRIGHT BLUE (darker)
	//0xED7EED: "95", // BRIGHT MAGENTA (darker)
	//0x00E5E5: "96", // BRIGHT CYAN (darker)
	//0xFFFFFF: "97", // BRIGHT WHITE (light gray)

	bgAnsi = make(map[uint32]string)

	blocks = []struct {
		Char rune
		Quad Quadrants
	}{
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
)

func init() {
	newAnsi := fgAnsi
	for overrideColor, code := range ansiOverrides {
		for origColor, origCode := range fgAnsi {
			if origCode == code {
				delete(newAnsi, origColor)
				newAnsi[overrideColor] = code
			}
		}
	}
	fgAnsi = newAnsi
	// If bgaAnsi is empty, populate it
	if len(bgAnsi) == 0 {
		for fgColor, code := range fgAnsi {
			if code[0] == '3' {
				bgAnsi[fgColor] = "4" + code[1:]
			} else if code[0] == '9' {
				bgAnsi[fgColor] = "10" + code[1:]
			}
		}
	}
	// Build reverse maps
	for c, code := range fgAnsi {
		fgAnsiRev[code] = c
	}
	for c, code := range bgAnsi {
		bgAnsiRev[code] = c
	}
}

// BlockChar represents a 2x2 block of characters with foreground and
// background colors mapped in the ANSI color space. The struct contains
// a rune representing the block character, and two RGB colors representing
// the foreground and background colors of the block.
type BlockChar struct {
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

// RGB represents a color in the RGB color space with 8-bit channels,
// where each channel ranges from 0 to 255. The RGB color space is
// additive, meaning that colors are created by adding together the
// red, green, and blue channels.
type RGB struct {
	r, g, b uint8
}

// toUint32 converts an RGB color to a 32-bit unsigned integer
func (r RGB) toUint32() uint32 {
	return uint32(r.r)<<16 | uint32(r.g)<<8 | uint32(r.b)
}

// rgbFromVecb converts a gocv.Vecb to an RGB color
func rgbFromVecb(color gocv.Vecb) RGB {
	return RGB{
		r: color[2],
		g: color[1],
		b: color[0],
	}
}

// rgbFromUint32 converts a 32-bit unsigned integer to an RGB color
func rgbFromUint32(color uint32) RGB {
	return RGB{
		r: uint8(color >> 16),
		g: uint8(color >> 8),
		b: uint8(color),
	}
}

// dithError calculates the error between two RGB colors in the RGB color
// space. It returns a 3-element array of floating-point numbers representing
// the error in the red, green, and blue channels, respectively.
func (r RGB) dithError(c2 RGB) [3]float64 {
	return [3]float64{
		float64(r.r) - float64(c2.r),
		float64(r.g) - float64(c2.g),
		float64(r.b) - float64(c2.b),
	}
}

// quantizeColor quantizes an RGB color by rounding each channel to the
// nearest multiple of the quantization factor. The function returns the
// quantized RGB color.
func (r RGB) quantizeColor() RGB {
	qFactor := 256 / float64(Quantization)
	return RGB{
		uint8(math.Round(float64(r.r)/qFactor) * qFactor),
		uint8(math.Round(float64(r.g)/qFactor) * qFactor),
		uint8(math.Round(float64(r.b)/qFactor) * qFactor),
	}
}

// subtract subtracts the RGB channels of another RGB color from the
// corresponding channels of the current RGB color. The function returns
// a new RGB color with the result of the subtraction.
func (r RGB) subtract(other RGB) RGB {
	return RGB{
		r: uint8(math.Max(0, float64(r.r)-float64(other.r))),
		g: uint8(math.Max(0, float64(r.g)-float64(other.g))),
		b: uint8(math.Max(0, float64(r.b)-float64(other.b))),
	}
}

// colorDistance calculates the Euclidean distance between two RGB colors
// in the RGB color space. The function returns the distance as a floating-
// point number.
func (r RGB) colorDistance(other RGB) float64 {
	return math.Sqrt(float64(
		(int(r.r)-int(other.r))*(int(r.r)-int(other.r)) +
			(int(r.g)-int(other.g))*(int(r.g)-int(other.g)) +
			(int(r.b)-int(other.b))*(int(r.b)-int(other.b)),
	))
}

// modifiedAtkinsonDitherForBlocks applies the modified Atkinson dithering
// algorithm to an image operating on 2x2 blocks rather than pixels. The
// function takes an input image and a binary image with edges detected. It
// returns a BlockChar representation with the Atkinson dithering algorithm
// applied, with colors quantized to the nearest ANSI color.
func modifiedAtkinsonDitherForBlocks(img gocv.Mat, edges gocv.Mat) [][]BlockChar {
	height, width := img.Rows(), img.Cols()
	blockHeight, blockWidth := height/2, width/2
	result := make([][]BlockChar, blockHeight)
	for i := range result {
		result[i] = make([]BlockChar, blockWidth)
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
			bestChar, fgColor, bgColor := findBestBlockRepresentation(block, isEdge)

			// Store the result
			result[by][bx] = BlockChar{
				Rune: bestChar,
				FG:   fgColor,
				BG:   bgColor,
			}

			// Calculate and distribute the error
			for i, blockColor := range block {
				y, x := by*2+i/2, bx*2+i%2
				var targetColor RGB
				if (bestChar & (1 << (3 - i))) != 0 {
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
	var bestChar rune
	var bestFG, bestBG RGB
	minError := math.MaxFloat64

	for _, b := range blocks {
		for fg := range fgAnsi {
			fgRgb := rgbFromUint32(fg)
			for bg := range bgAnsi {
				bgRgb := rgbFromUint32(bg)
				if fg == bg {
					continue
				}
				colorError := calculateBlockError(block, b.Quad, fgRgb, bgRgb, isEdge)
				if colorError < minError {
					minError = colorError
					bestChar = b.Char
					bestFG = fgRgb
					bestBG = bgRgb
				}
			}
		}
	}

	return bestChar, bestFG, bestBG
}

// calculateBlockError calculates the error between a 2x2 block of colors
// and a given representation of a block. The function takes the block of
// colors, the quadrants of the block representation, the foreground and
// background colors, and a boolean value indicating whether the block is
// an edge block. It returns the error as a floating-point number.
func calculateBlockError(block [4]RGB, quad Quadrants, fg, bg RGB, isEdge bool) float64 {
	var colorError float64
	quadrants := [4]bool{quad.TopLeft, quad.TopRight, quad.BottomLeft, quad.BottomRight}

	for i, blockColor := range block {
		var target RGB
		if quadrants[i] {
			target = fg
		} else {
			target = bg
		}
		colorError += blockColor.colorDistance(target)
	}

	if isEdge {
		colorError *= 0.5 // Reduce colorError for edge blocks to preserve edges
	}
	return colorError
}

// drawBlock draws a 2x2 block of a rune with the given foreground and
// background colors at the specified position in an image. The function
// takes a pointer to an image, the x and y coordinates of the block, and
// the block character to draw.
func drawBlock(img *image.RGBA, x, y int, block BlockChar) {
	quad := getQuadrantsForRune(block.Rune)
	quadrants := [4]bool{quad.TopLeft, quad.TopRight, quad.BottomLeft, quad.BottomRight}

	for i := 0; i < 4; i++ {
		dx, dy := i%2, i/2
		if quadrants[i] {
			img.Set(x+dx, y+dy, color.RGBA{block.FG.r, block.FG.g, block.FG.b, 255})
		} else {
			img.Set(x+dx, y+dy, color.RGBA{block.BG.r, block.BG.g, block.BG.b, 255})
		}
	}
}

// getQuadrantsForRune returns the quadrants for a given rune character.
// The function takes a rune character and returns the quadrants for the
// corresponding block character, or an empty Quadrants struct if the
// character is not found.
func getQuadrantsForRune(char rune) Quadrants {
	for _, b := range blocks {
		if b.Char == char {
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

// compressANSI compresses an ANSI image by combining adjacent blocks with
// the same foreground and background colors. The function takes an ANSI
// image as a string and returns the compressed ANSI image as a string.
func compressANSI(ansiImage string) string {
	var compressed strings.Builder
	var currentFg, currentBg, currentBlock string
	var count int

	lines := strings.Split(ansiImage, "\n")
	for _, line := range lines {
		segments := strings.Split(line, "\u001b[")
		for _, segment := range segments {
			if segment == "" {
				continue
			}
			parts := strings.SplitN(segment, "m", 2)
			if len(parts) != 2 {
				continue
			}
			colorCode, block := parts[0], parts[1]
			fg, bg := extractColors(colorCode)

			// Optimize full block representation
			if block == "█" {
				bg = ""
			} else if block == " " {
				fg = ""
			}

			if fg != currentFg || bg != currentBg || block != currentBlock {
				if count > 0 {
					compressed.WriteString(
						formatANSICode(currentFg, currentBg, currentBlock, count))
				}
				currentFg, currentBg, currentBlock = fg, bg, block
				count = 1
			} else {
				count++
			}
		}
		if count > 0 {
			compressed.WriteString(
				formatANSICode(currentFg, currentBg, currentBlock, count))
		}
		compressed.WriteString(fmt.Sprintf("%s[0m\n", ESC))
		count = 0
		currentFg, currentBg = "", "" // Reset colors at end of line
	}

	return compressed.String()
}

// formatANSICode formats an ANSI color code with the given foreground and
// background colors, block character, and count. The function returns the
// ANSI color code as a string, with the foreground and background colors
// formatted as ANSI color codes, the block character repeated count times.
func formatANSICode(fg, bg, block string, count int) string {
	var code strings.Builder
	code.WriteString(ESC)
	code.WriteByte('[')
	if fg != "" {
		code.WriteString(fg)
		if bg != "" {
			code.WriteByte(';')
		}
	}
	if bg != "" {
		code.WriteString(bg)
	}
	code.WriteByte('m')
	code.WriteString(strings.Repeat(block, count))
	return code.String()
}

// extractColors extracts the foreground and background color codes from
// an ANSI color code. The function takes an ANSI color code as a string
// and returns the foreground and background color codes as strings.
func extractColors(colorCodes string) (fg string, bg string) {
	colors := strings.Split(colorCodes, ";")
	for _, colorCode := range colors {
		if colorIsForeground(colorCode) {
			fg = colorCode
		} else if colorIsBackground(colorCode) {
			bg = colorCode
		}
	}
	return fg, bg
}

// saveBlocksToPNG saves a 2D array of BlockChar structs to a PNG file.
// The function takes a 2D array of BlockChar structs and a filename as
// strings, and returns an error if the file cannot be saved.
func saveBlocksToPNG(blocks [][]BlockChar, filename string) error {
	height, width := len(blocks)*2, len(blocks[0])*2
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y, row := range blocks {
		for x, block := range row {
			drawBlock(img, x*2, y*2, block)
		}
	}

	f, createErr := os.Create(filename)
	if createErr != nil {
		return createErr
	}
	defer func(f *os.File) {
		closeErr := f.Close()
		if closeErr != nil {
			println("Error closing block PNG file")
		}
	}(f)

	return png.Encode(f, img)
}

// saveToPNG saves an image to a PNG file. The function takes an image as
// a gocv.Mat and a filename as a string, and returns an error if the image
// cannot be saved.
func saveToPNG(img gocv.Mat, filename string) error {
	success := gocv.IMWrite(filename, img)
	if !success {
		return fmt.Errorf("failed to write image to file: %s", filename)
	}
	return nil
}

// colorIsForeground returns true if the ANSI color code corresponds to a
// foreground color, and false otherwise. The function takes an ANSI color
// code as a string and returns true if the color is a foreground color, and
// false if it is a background color.
func colorIsForeground(color string) bool {
	return strings.HasPrefix(color, "3") ||
		strings.HasPrefix(color, "9")
}

// colorIsBackground returns true if the ANSI color code corresponds to a
// background color, and false otherwise. The function takes an ANSI color
// code as a string and returns true if the color is a background color, and
// false if it is a foreground color.
func colorIsBackground(color string) bool {
	return strings.HasPrefix(color, "4") ||
		strings.HasPrefix(color, "10")
}

// renderToAnsi renders a 2D array of BlockChar structs to an ANSI string.
// It does not perform any compression or optimization.
func renderToAnsi(blocks [][]BlockChar) string {
	var sb strings.Builder

	for _, row := range blocks {
		for _, block := range row {
			fgCode := fgAnsi[block.FG.toUint32()]
			bgCode := bgAnsi[block.BG.toUint32()]

			sb.WriteString(fmt.Sprintf("\x1b[%s;%sm", fgCode, bgCode))
			sb.WriteRune(block.Rune)
		}
		// Reset colors at the end of each line and add a newline
		sb.WriteString("\x1b[0m\n")
	}

	return sb.String()
}

// prepareForANSI prepares an image for conversion to ANSI art. The function
// takes an input image, the target width and height for the output image, and
// returns the resized image and the edges detected in the image.
func prepareForANSI(img gocv.Mat, width, height int) (resized, edges gocv.Mat) {
	intermediate := gocv.NewMat()
	resized = gocv.NewMat()
	edges = gocv.NewMat()

	// Use area interpolation for downscaling to an intermediate size
	intermediateWidth := width * 4 // or another multiplier that gives results
	intermediateHeight := height * 4
	gocv.Resize(img,
		&intermediate,
		image.Point{X: intermediateWidth,
			Y: intermediateHeight},
		0, 0,
		gocv.InterpolationArea)

	// Detect edges on the intermediate image
	gray := gocv.NewMat()
	gocv.CvtColor(intermediate, &gray, gocv.ColorBGRToGray)
	gocv.Canny(gray, &edges, 50, 150) // Adjust thresholds as needed

	// Resize both the intermediate image and the edges to the final size
	resizedPoint := image.Point{X: width * 2, Y: height * 2}
	gocv.Resize(intermediate, &resized, resizedPoint, 0, 0,
		gocv.InterpolationArea)
	gocv.Resize(edges, &edges, resizedPoint, 0, 0,
		gocv.InterpolationLinear)

	// 4. Apply a very mild sharpening to the resized image
	kernel := gocv.NewMatWithSize(3, 3, gocv.MatTypeCV32F)
	kernel.SetFloatAt(0, 0, 0)
	kernel.SetFloatAt(0, 1, -0.5)
	kernel.SetFloatAt(0, 2, 0)
	kernel.SetFloatAt(1, 0, -0.5)
	kernel.SetFloatAt(1, 1, 3)
	kernel.SetFloatAt(1, 2, -0.5)
	kernel.SetFloatAt(2, 0, 0)
	kernel.SetFloatAt(2, 1, -0.5)
	kernel.SetFloatAt(2, 2, 0)

	sharpened := gocv.NewMat()
	gocv.Filter2D(resized,
		&sharpened, -1, kernel, image.Point{-1, -1},
		0, gocv.BorderDefault)
	resized = sharpened
	return resized, edges
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
	fgColors := make([]uint32, 0, len(fgAnsi))
	for fgColor := range fgAnsi {
		fgColors = append(fgColors, fgColor)
	}
	bgColors := make([]uint32, 0, len(bgAnsi))
	for bgColor := range bgAnsi {
		bgColors = append(bgColors, bgColor)
	}
	fmt.Printf("%17s", " ")
	for _, fg := range fgColors {
		fmt.Printf(" %6x (%3s) ", fg, fgAnsi[fg])
	}
	fmt.Println()
	for _, bg := range bgColors {
		fmt.Printf("   %6x (%3s) ", bg, bgAnsi[bg])

		for _, fg := range fgColors {
			fmt.Printf("    %s[%s;%sm %3s %3s %s[0m ",
				ESC, fgAnsi[fg], bgAnsi[bg], fgAnsi[fg], bgAnsi[bg], ESC)
		}
		fmt.Println()
	}
}

func main() {
	inputFile := flag.String("input", "", "Path to the input image file (required)")
	targetWidth := flag.Int("width", 100, "Target width of the output image")
	maxChars := flag.Int("maxchars", 1048576, "Maximum number of characters in the output")
	edgeDistance := flag.Uint("edge", 1250, "Edge distance")
	outputFile := flag.String("output", "", "Path to save the output (if not specified, prints to stdout)")
	quantization := flag.Int("quantization", 256, "Quantization factor")
	scaleFactor := flag.Float64("scale", 3.0, "Scale factor for the output image")

	// Parse flags
	flag.Parse()

	// Validate required flags
	if *inputFile == "" {
		fmt.Println("Please provide the path to the input image using the -input flag")
		flag.PrintDefaults()
		return
	}

	// Update global variables
	TargetWidth = *targetWidth
	MaxChars = *maxChars
	Quantization = *quantization
	ScaleFactor = *scaleFactor
	EdgeDistance = uint32(*edgeDistance)

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
	printAnsiTable()
}
