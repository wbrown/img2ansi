package main

import (
	"flag"
	"fmt"
	"gocv.io/x/gocv"
	"image"
	_ "image/png"
	"math"
	"os"
	"slices"
	"strings"
)

const (
	ESC = "\u001b"
)

var (
	TargetWidth  = 100
	ScaleFactor  = 3.0
	MaxChars     = 1048576
	Shading      = true
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
	}

	bgAnsi = make(map[uint32]string)

	blocks = []rune{
		' ', // 0000 - Empty space
		'▘', // 0001 - Quadrant upper left
		'▝', // 0010 - Quadrant upper right
		'▀', // 0011 - Upper half block
		'▖', // 0100 - Quadrant lower left
		'▌', // 0101 - Left half block
		'▞', // 0110 - Quadrant diagonal upper right and lower left
		'▛', // 0111 - Three quadrants: upper left, upper right, lower left
		'▗', // 1000 - Quadrant lower right
		'▚', // 1001 - Quadrant diagonal upper left and lower right
		'▐', // 1010 - Right half block
		'▜', // 1011 - Three quadrants: upper left, upper right, lower right
		'▄', // 1100 - Lower half block
		'▙', // 1101 - Three quadrants: upper left, lower left, lower right
		'▟', // 1110 - Three quadrants: upper right, lower left, lower right
		'█', // 1111 - Full block
	}
)

func init() {
	for color, code := range fgAnsi {
		if code[0] == '3' {
			bgAnsi[color] = "4" + code[1:]
		} else if code[0] == '9' {
			bgAnsi[color] = "10" + code[1:]
		}
	}
}

type RGB struct {
	r, g, b uint8
}

func (rgb RGB) toUint32() uint32 {
	return uint32(rgb.r)<<16 | uint32(rgb.g)<<8 | uint32(rgb.b)
}

func rgbFromVecb(color gocv.Vecb) RGB {
	return RGB{
		r: color[2],
		g: color[1],
		b: color[0],
	}
}

func rgbFromUint32(color uint32) RGB {
	return RGB{
		r: uint8(color >> 16),
		g: uint8(color >> 8),
		b: uint8(color),
	}
}

func (rgb RGB) rgbToANSI(fg bool) (RGB, string) {
	colorDict := fgAnsi
	if !fg {
		colorDict = bgAnsi
	}

	minDiff := uint32(math.MaxUint32)
	var bestCode string
	var bestColor RGB

	for k, v := range colorDict {
		diff := rgb.colorDistance(rgbFromUint32(k))
		if diff < minDiff {
			minDiff = diff
			bestCode = v
			bestColor = rgbFromUint32(k)
		}
	}

	return bestColor, bestCode
}

func (rgb RGB) colorDistance(c2 RGB) uint32 {
	return uint32(math.Pow(float64(rgb.r)-float64(c2.r), 2) +
		math.Pow(float64(rgb.g)-float64(c2.g), 2) +
		math.Pow(float64(rgb.b)-float64(c2.b), 2))
}

func (rgb RGB) dithError(c2 RGB) [3]float64 {
	return [3]float64{
		float64(rgb.r) - float64(c2.r),
		float64(rgb.g) - float64(c2.g),
		float64(rgb.b) - float64(c2.b),
	}
}

func detectEdges(img gocv.Mat) gocv.Mat {
	gray := gocv.NewMat()
	edges := gocv.NewMat()
	kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(3, 3))

	gocv.CvtColor(img, &gray, gocv.ColorRGBToGray)
	gocv.Canny(gray, &edges, 50, 150)
	gocv.Dilate(edges, &edges, kernel)

	return edges
}

func (rgb RGB) quantizeColor() RGB {
	qFactor := 256 / float64(Quantization)
	return RGB{
		uint8(math.Round(float64(rgb.r)/qFactor) * qFactor),
		uint8(math.Round(float64(rgb.g)/qFactor) * qFactor),
		uint8(math.Round(float64(rgb.b)/qFactor) * qFactor),
	}
}

func modifiedAtkinsonDither(img gocv.Mat, edges gocv.Mat) gocv.Mat {
	height, width := img.Rows(), img.Cols()
	newImage := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8UC3)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			oldPixel := rgbFromVecb(img.GetVecbAt(y, x))
			quantizedPixel := oldPixel.quantizeColor()
			newColor, _ := quantizedPixel.rgbToANSI(true)
			// Store full color information
			newImage.SetUCharAt(y, x*3+2, newColor.r)
			newImage.SetUCharAt(y, x*3+1, newColor.g)
			newImage.SetUCharAt(y, x*3, newColor.b)

			if edges.GetUCharAt(y, x) > 0 || quantizedPixel.colorDistance(newColor) < 1250 {
				continue
			}

			dithError := oldPixel.dithError(newColor)

			for i := 0; i < 3; i++ {
				dithError[i] /= 8
			}
			diffuseError := func(y, x int, factor float64) {
				if y >= 0 && y < height && x >= 0 && x < width {
					pixel := img.GetVecbAt(y, x)
					for i := 0; i < 3; i++ {
						newVal := uint8(math.Max(0, math.Min(255, float64(pixel[2-i])+dithError[i]*factor)))
						img.SetUCharAt(y, x*3+(2-i), newVal)
					}
				}
			}

			diffuseError(y, x+1, 1)
			diffuseError(y, x+2, 1)
			diffuseError(y+1, x, 1)
			diffuseError(y+1, x+1, 1)
			diffuseError(y+1, x-1, 1)
			diffuseError(y+2, x, 1)
		}
	}

	return newImage
}

func colorFromANSI(ansiCode string) RGB {
	var table map[uint32]string
	if colorIsForeground(ansiCode) {
		table = fgAnsi
	} else {
		table = bgAnsi
	}
	for color, code := range table {
		if code == ansiCode {
			return rgbFromUint32(color)
		}
	}
	return RGB{}
}

func getANSICode(img gocv.Mat, y, x int) string {
	rgb := RGB{img.GetUCharAt(y, x*3+2),
		img.GetUCharAt(y, x*3+1),
		img.GetUCharAt(y, x*3)}
	_, ansiCode := rgb.rgbToANSI(true)
	return ansiCode
}

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

			if fg != currentFg || bg != currentBg || block != currentBlock {
				if count > 0 {
					compressed.WriteString(formatANSICode(currentFg, currentBg, currentBlock, count))
				}
				currentFg, currentBg, currentBlock = fg, bg, block
				count = 1
			} else {
				count++
			}
		}
		if count > 0 {
			compressed.WriteString(formatANSICode(currentFg, currentBg, currentBlock, count))
		}
		compressed.WriteString(fmt.Sprintf("%s[0m\n", ESC))
		count = 0
		currentFg, currentBg = "", "" // Reset colors at end of line
	}

	return compressed.String()
}

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

func extractColors(colorCode string) (string, string) {
	colors := strings.Split(colorCode, ";")
	var fg, bg string
	for _, color := range colors {
		if strings.HasPrefix(color, "3") {
			fg = color
		} else if strings.HasPrefix(color, "4") {
			bg = color
		}
	}
	return fg, bg
}

func saveToPNG(img gocv.Mat, filename string) error {
	success := gocv.IMWrite(filename, img)
	if !success {
		return fmt.Errorf("failed to write image to file: %s", filename)
	}
	return nil
}

func getBlock(upperLeft, upperRight, lowerLeft, lowerRight bool) rune {
	index := 0
	if upperLeft {
		index |= 1
	}
	if upperRight {
		index |= 2
	}
	if lowerLeft {
		index |= 4
	}
	if lowerRight {
		index |= 8
	}
	return blocks[index]
}

func averageColor(colors []string) string {
	var r, g, b, count float64
	for _, colorStr := range colors {
		color := colorFromANSI(colorStr)
		r += float64(color.r & 0xFF)
		g += float64(color.g & 0xFF)
		b += float64(color.b & 0xFF)
		count++
	}
	avgRgb := RGB{
		uint8(r / count),
		uint8(g / count),
		uint8(b / count),
	}
	_, ansiCode := avgRgb.rgbToANSI(true)
	return ansiCode
}

func colorIsForeground(color string) bool {
	return strings.HasPrefix(color, "3") || strings.HasPrefix(color, "9")
}

func colorIsBackground(color string) bool {
	return strings.HasPrefix(color, "4") || strings.HasPrefix(color, "10")
}

func chooseColorsFromNeighborhood(img gocv.Mat, y, x int) (string, string) {
	neighborhoodSize := 5
	colorCounts := make(map[string]int)

	for dy := -neighborhoodSize / 2; dy <= neighborhoodSize/2; dy++ {
		for dx := -neighborhoodSize / 2; dx <= neighborhoodSize/2; dx++ {
			ny, nx := y+dy*2, x+dx*2
			if ny >= 0 && ny < img.Rows() && nx >= 0 && nx < img.Cols() {
				color := getANSICode(img, ny, nx)
				colorCounts[color]++
			}
		}
	}

	sortedColors := getMostFrequentColors(mapToSlice(colorCounts))
	if len(sortedColors) == 1 {
		return sortedColors[0], sortedColors[0]
	}

	firstColorIsFg := colorIsForeground(sortedColors[0])
	secondColorIsFg := colorIsForeground(sortedColors[1])
	if firstColorIsFg == secondColorIsFg {
		bgOrFg := !(firstColorIsFg && secondColorIsFg)
		secondColorCandidate := colorFromANSI(sortedColors[1])
		_, sortedColors[1] = secondColorCandidate.rgbToANSI(bgOrFg)
	}
	return sortedColors[0], sortedColors[1]
}

func resolveColorPair(first, second string) (string, string) {
	firstColorIsFg := colorIsForeground(first)
	secondColorIsFg := colorIsForeground(second)
	if firstColorIsFg == secondColorIsFg {
		bgOrFg := !(firstColorIsFg && secondColorIsFg)
		secondColorCandidate := colorFromANSI(second)
		_, second = secondColorCandidate.rgbToANSI(bgOrFg)
	}
	if colorIsForeground(first) && colorIsBackground(second) {
		return first, second
	} else if colorIsForeground(second) && colorIsBackground(first) {
		return second, first
	} else if colorIsForeground(first) {
		return first, second
	} else if colorIsForeground(second) {
		return first, second
	}
	return first, second
}

func getBlockFromColors(colors []string, fgColor, bgColor string) rune {
	return getBlock(
		colors[0] == fgColor,
		colors[1] == fgColor,
		colors[2] == fgColor,
		colors[3] == fgColor,
	)
}

func mapToSlice(m map[string]int) []string {
	result := make([]string, 0, len(m))
	for k, ct := range m {
		for i := 0; i < ct; i++ {
			result = append(result, k)
		}
	}
	return result
}

func simpleResolveBlock(colors []string) (fgColor, bgColor string, ansiBlock rune) {
	// Handle the two-color case (or single color)
	dominantColors := getMostFrequentColors(colors)
	fgColor = dominantColors[0]
	var block rune

	if len(dominantColors) > 1 {
		bgCandidate := colorFromANSI(dominantColors[1])
		bgColor = bgAnsi[bgCandidate.toUint32()]
		block = getBlock(
			colors[0] == fgColor,
			colors[1] == fgColor,
			colors[2] == fgColor,
			colors[3] == fgColor,
		)
	} else {
		bgColor = "40" // Default to black background for single color
		block = '█'    // Full block
	}
	return fgColor, bgColor, block
}

func resolveBlock(colors []string) (fgColor, bgColor string, ansiBlock rune) {
	uniqueColors := make(map[string]int)
	for _, c := range colors {
		uniqueColors[c]++
	}

	//if len(uniqueColors) > 2 && Shading {
	//	// Handle the three-or-more-color case
	//	fgColor, bgColor := chooseColorsFromNeighborhood(ditheredImg, y, x)
	//	block := getBlockFromColors(colors, fgColor, bgColor)
	//	ansiImage += fmt.Sprintf("%s[%s;%sm%s", ESC, fgColor, bgAnsi[colorFromANSI(bgColor)], string(block))
	// We need to resolve the following scenarios:
	// 1. >1 colors are foreground, >1 are background
	// 2. Three or four different colors
	//block := mergeBlockColors(colors)
	block := colors

	// At this point, we should only have one or two colors
	dominantColors := getMostFrequentColors(block)
	fgColor = dominantColors[0]

	if len(dominantColors) > 1 {
		fgColor, bgColor = resolveColorPair(dominantColors[0],
			dominantColors[1])
		ansiBlock = getBlock(
			colors[0] == fgColor,
			colors[1] == fgColor,
			colors[2] == fgColor,
			colors[3] == fgColor,
		)
	} else {
		fgColor = dominantColors[0]
		ansiBlock = '█' // Full block
	}
	return fgColor, bgColor, ansiBlock
}

func imageToANSI(imagePath string) string {
	img := gocv.IMRead(imagePath, gocv.IMReadAnyColor)
	if img.Empty() {
		return fmt.Sprintf("Could not read image from %s", imagePath)
	}
	defer img.Close()

	aspectRatio := float64(img.Cols()) / float64(img.Rows())
	width := TargetWidth
	height := int(float64(width) / aspectRatio / ScaleFactor)

	for {
		resized := gocv.NewMat()
		gocv.Resize(img, &resized, image.Point{X: width * 2, Y: height * 2}, 0, 0, gocv.InterpolationLinear)

		edges := detectEdges(resized)
		ditheredImg := modifiedAtkinsonDither(resized, edges)
		// Write the dithered image to a file for debugging
		saveToPNG(ditheredImg, "dithered.png")

		ansiImage := ""

		imgHeight, imgWidth := ditheredImg.Rows(), ditheredImg.Cols()

		for y := 0; y < imgHeight; y += 2 {
			for x := 0; x < imgWidth; x += 2 {
				colors := []string{
					getANSICode(ditheredImg, y, x),
					getANSICode(ditheredImg, y, x+1),
					getANSICode(ditheredImg, y+1, x),
					getANSICode(ditheredImg, y+1, x+1),
				}

				uniqueColors := make(map[string]int)
				for _, c := range colors {
					uniqueColors[c]++
				}

				var fgColor, bgColor string
				var ansiStr string
				if len(uniqueColors) > 2 {
					// Handle the three-or-more-color case
					fgColor, bgColor = chooseColorsFromNeighborhood(ditheredImg, y, x)
					ansiStr = string(getBlockFromColors(colors, fgColor, bgColor))
					bgRgb := colorFromANSI(bgColor)
					bgColor = bgAnsi[bgRgb.toUint32()]
				} else {
					var ansiBlock rune
					fgColor, bgColor, ansiBlock = simpleResolveBlock(colors)
					ansiStr = string(ansiBlock)
				}
				ansiImage += fmt.Sprintf("%s[%s;%sm%s", ESC, fgColor, bgColor, ansiStr)
			}

			ansiImage += fmt.Sprintf("%s[0m\n", ESC)
		}

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

func getMostFrequentColors(colors []string) []string {
	colorCount := make(map[string]int)
	for _, color := range colors {
		colorCount[color]++
	}
	// Sort the colors by frequency -- first create tuples
	type colorTuple struct {
		color string
		count int
	}
	colorTuples := make([]colorTuple, 0, len(colorCount))
	for color, count := range colorCount {
		colorTuples = append(colorTuples, colorTuple{color, count})
	}
	// Sort the tuples by count
	slices.SortFunc(colorTuples, func(a colorTuple, j colorTuple) int {
		return j.count - a.count
	})

	// Extract the colors from the sorted tuples
	dominantColors := make([]string, 0, len(colorTuples))
	for _, tuple := range colorTuples {
		dominantColors = append(dominantColors, tuple.color)
	}

	return dominantColors
}

func mergeColors(block []string) []string {
	for idx := 0; idx < len(block)-1; idx++ {
		firstColor := colorFromANSI(block[idx])
		secondColor := colorFromANSI(block[idx+1])
		if firstColor == secondColor {
			block[idx+1] = block[idx]
		}
	}
	return block
}

func mergeBlockColors(block []string) []string {
	// We need to scan our frequent colors,  determine if there are
	// colors that are the same, and merge them
	rgbColors := make([]RGB, 0, len(block))
	for idx := 0; idx < len(block); idx++ {
		rgbColors = append(rgbColors, colorFromANSI(block[idx]))
	}
	// Now that we've mapped to actual RGB colors, we can determine
	// if we have two or more colors that are the same
	for idx := 0; idx < len(rgbColors); idx++ {
		for jdx := idx + 1; jdx < len(rgbColors); jdx++ {
			if jdx == idx {
				continue
			}
			if rgbColors[idx].toUint32() == rgbColors[jdx].toUint32() {
				block[jdx] = block[idx]
			} else {
				block[jdx] = block[jdx]
			}
		}
	}

	block = mergeColors(block)
	freqColors := getMostFrequentColors(block)
	if len(freqColors) == 1 {
		return block
	} else {
		block = mergeColors(block)
		freqColors = getMostFrequentColors(block)
		if len(freqColors) == 1 {
			return block
		}
		if len(freqColors) == 2 {
			if colorIsForeground(freqColors[0]) && colorIsBackground(freqColors[1]) {
				return block
			} else if colorIsForeground(freqColors[1]) && colorIsBackground(freqColors[0]) {
				return block
			}
		}

		freqColors = getMostFrequentColors(block)

		fg, bg := resolveColorPair(freqColors[0], freqColors[1])
		for idx := 0; idx < len(block); idx++ {
			if block[idx] == freqColors[0] {
				block[idx] = fg
			} else if block[idx] == freqColors[1] {
				block[idx] = bg
			}
		}
		block = mergeColors(block)
		freqColors = getMostFrequentColors(block)
		if len(freqColors) < 3 {
			return block
		} else {
			fg, bg = resolveColorPair(freqColors[0], freqColors[1])
			// We still have three or more colors, so we need to perform
			// a nearest color match to either fg or bg
			for idx := 0; idx < len(block); idx++ {
				currAnsi := block[idx]
				currColor := colorFromANSI(currAnsi)
				if currAnsi == fg || currAnsi == bg {
					continue
				}
				// Determine if currAnsi is closer to fg or bg
				fgColor := colorFromANSI(fg)
				bgColor := colorFromANSI(bg)
				fgDist := fgColor.colorDistance(currColor)
				bgDist := bgColor.colorDistance(currColor)
				if fgDist < bgDist {
					block[idx] = fg
				} else {
					block[idx] = bg
				}
			}
		}
	}
	return block
}

func main() {
	inputFile := flag.String("input", "", "Path to the input image file (required)")
	targetWidth := flag.Int("width", 100, "Target width of the output image")
	maxChars := flag.Int("maxchars", 1048576, "Maximum number of characters in the output")
	enableShading := flag.Bool("shading", false, "Enable shading for more detailed output")
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
	Shading = *enableShading
	Quantization = *quantization
	ScaleFactor = *scaleFactor

	if len(os.Args) < 2 {
		fmt.Println("Please provide the path to the image as an argument")
		return
	}

	// Generate ANSI art
	ansiArt := imageToANSI(*inputFile)
	//compressedArt := compressANSI(ansiArt)
	compressedArt := ansiArt

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
}
