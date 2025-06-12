//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"gocv.io/x/gocv"
	"io/ioutil"
	"log"

	"github.com/wbrown/img2ansi"
)

func main() {
	// Load palette
	img2ansi.CurrentColorDistanceMethod = img2ansi.MethodLAB
	_, _, err := img2ansi.LoadPalette("colordata/ansi16.json")
	if err != nil {
		log.Fatalf("Failed to load palette: %v", err)
	}

	fmt.Println("Generating ANSI output comparisons...")

	// Create a gradient test image
	width, height := 32, 16
	img := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8UC3)
	defer img.Close()

	// Create a diagonal gradient
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Diagonal gradient from dark to light
			val := uint8((x + y) * 255 / (width + height))
			img.SetUCharAt(y, x*3, val)   // B
			img.SetUCharAt(y, x*3+1, val) // G
			img.SetUCharAt(y, x*3+2, val) // R
		}
	}

	edges := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8U)
	defer edges.Close()

	// Process with current implementation (bitwise bug)
	imgCopy1 := img.Clone()
	defer imgCopy1.Close()
	blocksBitwise := img2ansi.BrownDitherForBlocks(imgCopy1, edges)

	// Generate ANSI output
	ansiBitwise := img2ansi.RenderToAnsi(blocksBitwise)
	err = ioutil.WriteFile("gradient_bitwise.ans", []byte(ansiBitwise), 0644)
	if err != nil {
		log.Fatalf("Failed to write bitwise ANSI: %v", err)
	}
	fmt.Println("Created gradient_bitwise.ans")

	// Also save as PNG for reference
	err = img2ansi.SaveBlocksToPNGWithOptions(blocksBitwise, "gradient_bitwise.png", img2ansi.RenderOptions{
		UseFont: false,
		Scale:   4,
	})
	if err != nil {
		log.Fatalf("Failed to save bitwise PNG: %v", err)
	}

	// Process without error diffusion for comparison
	imgCopy2 := img.Clone()
	defer imgCopy2.Close()
	blocksNoDiff := brownDitherNoDiffusion(imgCopy2, edges)

	ansiNoDiff := img2ansi.RenderToAnsi(blocksNoDiff)
	err = ioutil.WriteFile("gradient_no_diffusion.ans", []byte(ansiNoDiff), 0644)
	if err != nil {
		log.Fatalf("Failed to write no diffusion ANSI: %v", err)
	}
	fmt.Println("Created gradient_no_diffusion.ans")

	// Create a more complex test pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Checkerboard with color variation
			if (x/4+y/4)%2 == 0 {
				// Brown-ish colors
				img.SetUCharAt(y, x*3, uint8(50+x*2))    // B
				img.SetUCharAt(y, x*3+1, uint8(80+x*2))  // G
				img.SetUCharAt(y, x*3+2, uint8(150+x*2)) // R
			} else {
				// Gray-ish colors
				val := uint8(100 + y*5)
				img.SetUCharAt(y, x*3, val)
				img.SetUCharAt(y, x*3+1, val)
				img.SetUCharAt(y, x*3+2, val)
			}
		}
	}

	imgCopy3 := img.Clone()
	defer imgCopy3.Close()
	blocksPattern := img2ansi.BrownDitherForBlocks(imgCopy3, edges)

	ansiPattern := img2ansi.RenderToAnsi(blocksPattern)
	err = ioutil.WriteFile("pattern_bitwise.ans", []byte(ansiPattern), 0644)
	if err != nil {
		log.Fatalf("Failed to write pattern ANSI: %v", err)
	}
	fmt.Println("Created pattern_bitwise.ans")

	// Create a simple test showing the 16 block characters
	blockDemo := make([][]img2ansi.BlockRune, 2)
	chars := []rune{
		' ', '▘', '▝', '▀', '▖', '▌', '▞', '▛',
		'▗', '▚', '▐', '▜', '▄', '▙', '▟', '█',
	}

	for i := 0; i < 2; i++ {
		blockDemo[i] = make([]img2ansi.BlockRune, 8)
		for j := 0; j < 8; j++ {
			idx := i*8 + j
			if idx < len(chars) {
				blockDemo[i][j] = img2ansi.BlockRune{
					Rune: chars[idx],
					FG:   img2ansi.RGB{255, 255, 255}, // White
					BG:   img2ansi.RGB{0, 0, 0},       // Black
				}
			}
		}
	}

	ansiDemo := img2ansi.RenderToAnsi(blockDemo)
	err = ioutil.WriteFile("block_characters_demo.ans", []byte(ansiDemo), 0644)
	if err != nil {
		log.Fatalf("Failed to write demo ANSI: %v", err)
	}
	fmt.Println("Created block_characters_demo.ans")

	fmt.Println("\nYou can now view these .ans files with:")
	fmt.Println("  cat gradient_bitwise.ans")
	fmt.Println("  cat gradient_no_diffusion.ans")
	fmt.Println("  cat pattern_bitwise.ans")
	fmt.Println("  cat block_characters_demo.ans")
	fmt.Println("\nOr use an ANSI art viewer for better display")
}

// brownDitherNoDiffusion is a copy without error diffusion
func brownDitherNoDiffusion(img gocv.Mat, edges gocv.Mat) [][]img2ansi.BlockRune {
	height, width := img.Rows(), img.Cols()
	blockHeight, blockWidth := height/2, width/2
	result := make([][]img2ansi.BlockRune, blockHeight)

	for by := range result {
		result[by] = make([]img2ansi.BlockRune, blockWidth)
		for bx := range result[by] {
			var block [4]img2ansi.RGB
			for i := 0; i < 4; i++ {
				y, x := by*2+i/2, bx*2+i%2
				if y < height && x < width {
					vecb := img.GetVecbAt(y, x)
					block[i] = img2ansi.RGB{R: vecb[2], G: vecb[1], B: vecb[0]}
				}
			}

			isEdge := false
			for i := 0; i < 4; i++ {
				y, x := by*2+i/2, bx*2+i%2
				if y < height && x < width && edges.GetUCharAt(y, x) > 0 {
					isEdge = true
					break
				}
			}

			bestRune, fgColor, bgColor := findBestBlockRepresentation(block, isEdge)

			result[by][bx] = img2ansi.BlockRune{
				Rune: bestRune,
				FG:   fgColor,
				BG:   bgColor,
			}
		}
	}

	return result
}
