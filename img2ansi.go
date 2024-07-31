package main

import (
	"fmt"
	"gocv.io/x/gocv"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"os"
)

const (
	ESC         = "\u001b"
	TargetWidth = 160
	MaxChars    = 128000
)

var (
	fgDiscord = map[uint32]string{
		0x000000: "30", // BLACK
		0xFF0000: "31", // RED
		0x00FF00: "32", // GREEN
		0xFFFF00: "33", // YELLOW
		0x0000FF: "34", // BLUE
		0xFF00FF: "35", // MAGENTA
		0x00FFFF: "36", // CYAN
		0xFFFFFF: "37", // WHITE
	}

	bgDiscord = make(map[uint32]string)

	blocks = []rune{' ', '▘', '▝', '▀', '▖', '▌', '▞', '▛', '▗', '▚', '▐', '▜', '▄', '▙', '▟', '█'}
)

func init() {
	for color, code := range fgDiscord {
		bgDiscord[color] = "4" + code[1:]
	}
}

func rgbToANSI(r, g, b uint8, fg bool) (uint32, string) {
	color := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	colorDict := fgDiscord
	if !fg {
		colorDict = bgDiscord
	}

	minDiff := uint32(math.MaxUint32)
	var bestCode string
	var bestColor uint32

	for k, v := range colorDict {
		diff := colorDistance(color, k)
		if diff < minDiff {
			minDiff = diff
			bestCode = v
			bestColor = k
		}
	}

	return bestColor, bestCode
}

func colorDistance(c1, c2 uint32) uint32 {
	r1, g1, b1 := uint8(c1>>16), uint8(c1>>8), uint8(c1)
	r2, g2, b2 := uint8(c2>>16), uint8(c2>>8), uint8(c2)
	return uint32(math.Pow(float64(r1)-float64(r2), 2) +
		math.Pow(float64(g1)-float64(g2), 2) +
		math.Pow(float64(b1)-float64(b2), 2))
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

func quantizeColor(c color.RGBA, quant int) color.RGBA {
	qFactor := 256 / float64(quant)
	return color.RGBA{
		R: uint8(math.Round(float64(c.R)/qFactor) * qFactor),
		G: uint8(math.Round(float64(c.G)/qFactor) * qFactor),
		B: uint8(math.Round(float64(c.B)/qFactor) * qFactor),
		A: c.A,
	}
}

func modifiedAtkinsonDither(img gocv.Mat, edges gocv.Mat) gocv.Mat {
	height, width := img.Rows(), img.Cols()
	newImage := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8UC3)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			oldPixel := img.GetVecbAt(y, x)
			r, g, b := oldPixel[2], oldPixel[1], oldPixel[0]

			newColor, _ := rgbToANSI(r, g, b, true)
			// Store full color information
			newImage.SetUCharAt(y, x*3+2, uint8(newColor>>16))
			newImage.SetUCharAt(y, x*3+1, uint8(newColor>>8))
			newImage.SetUCharAt(y, x*3, uint8(newColor&0xFF))

			if oldPixel[0] != 0 && oldPixel[1] != 0 && oldPixel[2] != 0 {
				fmt.Printf("AtkinsonDither: Pixel at (%d, %d): Original RGB(%d,%d,%d), Stored value: %d\n", x, y, r, g, b, newColor)
			}

			if edges.GetUCharAt(y, x) > 0 || colorDistance(uint32(r)<<16|uint32(g)<<8|uint32(b), newColor) < 1250 {
				continue
			}

			dithError := [3]float64{
				float64(r) - float64((newColor>>16)&0xFF),
				float64(g) - float64((newColor>>8)&0xFF),
				float64(b) - float64(newColor&0xFF),
			}

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

func colorFromANSI(ansiCode string) uint32 {
	for color, code := range fgDiscord {
		if code == ansiCode {
			return color
		}
	}
	return 0 // Default to black if not found
}

func getANSICode(img gocv.Mat, y, x int) string {
	r := img.GetUCharAt(y, x*3+2)
	g := img.GetUCharAt(y, x*3+1)
	b := img.GetUCharAt(y, x*3)
	_, ansiCode := rgbToANSI(r, g, b, true)
	if r != 0 && g != 0 && b != 0 {
		println("getANSICode: Pixel at (%d, %d): RGB(%d,%d,%d), ANSI: %s\n", x, y, r, g, b, ansiCode)
	}
	return ansiCode
}

func saveToPNG(img gocv.Mat, filename string) error {
	success := gocv.IMWrite(filename, img)
	if !success {
		return fmt.Errorf("failed to write image to file: %s", filename)
	}
	return nil
}

func imageToANSI(imagePath string) string {
	img := gocv.IMRead(imagePath, gocv.IMReadAnyColor)
	if img.Empty() {
		return fmt.Sprintf("Could not read image from %s", imagePath)
	}
	defer img.Close()

	aspectRatio := float64(img.Cols()) / float64(img.Rows())
	width := TargetWidth
	height := int(float64(width) / aspectRatio / 2)

	for {
		resized := gocv.NewMat()
		gocv.Resize(img, &resized, image.Point{X: width, Y: height * 2}, 0, 0, gocv.InterpolationLinear)
		// Save the resized image as PNG
		err := saveToPNG(resized, "resized_output.png")

		edges := detectEdges(resized)
		ditheredImg := modifiedAtkinsonDither(resized, edges)

		// Save the dithered image as PNG
		err = saveToPNG(ditheredImg, "dithered_output.png")
		if err != nil {
			fmt.Printf("Error saving dithered image: %v\n", err)
		} else {
			fmt.Println("Dithered image saved as dithered_output.png")
		}

		ansiImage := ""
		var currentFg, currentBg string

		imgHeight, imgWidth := ditheredImg.Rows(), ditheredImg.Cols()

		for y := 0; y < imgHeight; y += 2 {
			for x := 0; x < imgWidth; x++ {
				upperColor := getANSICode(ditheredImg, y, x)
				lowerColor := ""
				if y+1 < imgHeight {
					lowerColor = getANSICode(ditheredImg, y+1, x)
				} else {
					lowerColor = upperColor // Use upper color if we're at the bottom edge
				}

				if upperColor == lowerColor {
					if currentFg != upperColor {
						ansiImage += fmt.Sprintf("%s[%sm", ESC, upperColor)
						currentFg = upperColor
					}
					ansiImage += "█"
				} else {
					if currentFg != upperColor {
						ansiImage += fmt.Sprintf("%s[%sm", ESC, upperColor)
						currentFg = upperColor
					}
					if currentBg != lowerColor {
						ansiImage += fmt.Sprintf("%s[%sm", ESC, bgDiscord[colorFromANSI(lowerColor)])
						currentBg = lowerColor
					}
					ansiImage += "▀"
				}
			}

			ansiImage += fmt.Sprintf("%s[0m\n", ESC)
			currentFg = ""
			currentBg = ""
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

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide the path to the image as an argument")
		return
	}

	imagePath := os.Args[1]
	ansiArt := imageToANSI(imagePath)
	fmt.Print(ansiArt)
	fmt.Printf("Total string length: %d\n", len(ansiArt))
}
