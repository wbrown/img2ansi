package main

import (
	"fmt"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"image"
	"io/ioutil"
	"math/bits"
	"math/rand"
	"strings"
	"unicode"
)

const (
	GlyphWidth  = 8
	GlyphHeight = 8
)

var specialGlyphs = map[rune]uint32{}

type GlyphBitmap uint64

type GlyphInfo struct {
	Rune       rune
	Img        *image.Alpha
	Bounds     fixed.Rectangle26_6
	Advance    fixed.Int26_6
	Bitmap     GlyphBitmap
	Weight     uint8             // Hamming weight
	RowWeights [GlyphHeight]byte // Weight of each row
}

type GlyphLookup struct {
	Glyphs    []GlyphInfo
	WeightMap [GlyphWidth*GlyphHeight + 1][]int // Map from weight to indices in Glyphs
	BitmapMap map[GlyphBitmap]int               // Map from bitmap to index in Glyphs
}

func NewGlyphLookup(glyphs []GlyphInfo) *GlyphLookup {
	gl := &GlyphLookup{
		Glyphs:    glyphs,
		WeightMap: [65][]int{},
		BitmapMap: make(map[GlyphBitmap]int),
	}

	for i, glyph := range glyphs {
		gl.WeightMap[glyph.Weight] = append(gl.WeightMap[glyph.Weight], i)
		gl.BitmapMap[glyph.Bitmap] = i
	}

	return gl
}

func (gl *GlyphLookup) FindClosestGlyph(block GlyphBitmap) GlyphInfo {
	weight := uint8(block.popCount())
	candidates := gl.WeightMap[weight]

	if len(candidates) == 0 {
		// If no exact weight match, find the closest weight
		lowerWeight, upperWeight := weight, weight
		for lowerWeight > 0 || upperWeight < 64 {
			if lowerWeight > 0 {
				lowerWeight--
				if len(gl.WeightMap[lowerWeight]) > 0 {
					candidates = gl.WeightMap[lowerWeight]
					break
				}
			}
			if upperWeight < 64 {
				upperWeight++
				if len(gl.WeightMap[upperWeight]) > 0 {
					candidates = gl.WeightMap[upperWeight]
					break
				}
			}
		}
	}

	bestMatch := candidates[0]
	minDiff := uint8(64) // Maximum possible difference

	for _, idx := range candidates {
		xorBitmap := block ^ gl.Glyphs[idx].Bitmap
		diff := uint8(xorBitmap.popCount())
		if diff < minDiff {
			minDiff = diff
			bestMatch = idx
		}
	}

	return gl.Glyphs[bestMatch]
}

var popCountTable [256]byte

func init() {
	for i := range popCountTable {
		popCountTable[i] = byte(bits.OnesCount8(uint8(i)))
	}
}

func (g GlyphBitmap) popCount() uint8 {
	return uint8(bits.OnesCount64(uint64(g)))
}

func loadFont(path string) (*truetype.Font, error) {
	fontData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	font, err := truetype.Parse(fontData)
	if err != nil {
		return nil, err
	}
	return font, nil
}

func (g GlyphBitmap) String() string {
	var sb strings.Builder

	// Add hex representation
	sb.WriteString(" ")
	for x := 0; x < GlyphWidth; x++ {
		sb.WriteString(fmt.Sprintf("%d", x))
	}
	for y := 0; y < GlyphHeight; y++ {
		sb.WriteString(fmt.Sprintf("\n%d", y))
		for x := 0; x < GlyphWidth; x++ {
			if g&(1<<(63-y*GlyphWidth-x)) != 0 {
				sb.WriteString("█")
			} else {
				sb.WriteString("·")
			}
		}
	}
	return sb.String()
}

func getSafeRunes() []rune {
	var safeRunes []rune

	// ASCII
	for r := rune(0x0000); r <= 0x007F; r++ {
		safeRunes = append(safeRunes, r)
	}

	// Latin-1 Supplement
	for r := rune(0x0080); r <= 0x00FF; r++ {
		safeRunes = append(safeRunes, r)
	}

	// Latin Extended-A
	for r := rune(0x0100); r <= 0x017F; r++ {
		safeRunes = append(safeRunes, r)
	}

	// General Punctuation
	for r := rune(0x2000); r <= 0x206F; r++ {
		safeRunes = append(safeRunes, r)
	}

	// Box Drawing Characters
	for r := rune(0x2500); r <= 0x257F; r++ {
		safeRunes = append(safeRunes, r)
	}

	// Block Elements
	for r := rune(0x2580); r <= 0x259F; r++ {
		safeRunes = append(safeRunes, r)
	}

	return safeRunes
}

func (info *GlyphInfo) analyzeGlyph() {
	img := info.Img
	var bitmap GlyphBitmap

	for y := 0; y < GlyphHeight; y++ {
		var rowWeight byte
		for x := 0; x < GlyphWidth; x++ {
			if img.AlphaAt(x, y).A > 64 {
				// Adjust bit position:
				// y counts from top to bottom (0 to GlyphHeight-1)
				// x counts from left to right (0 to GlyphWidth-1)
				// We need to reverse the bit order for each row, but
				// keep the row order as is
				heightShift := GlyphHeight - 1 - y
				widthShift := GlyphWidth - 1 - x
				bitmap |= 1 << (heightShift*GlyphWidth + widthShift)
				rowWeight++
			}
		}
		info.RowWeights[y] = rowWeight
		info.Weight += rowWeight
	}
	info.Bitmap = bitmap
}

func renderGlyph(ttf *truetype.Font, r rune) *GlyphInfo {
	face := truetype.NewFace(ttf, &truetype.Options{
		Size:    float64(GlyphHeight),
		DPI:     72,
		Hinting: font.HintingFull,
	})

	img := image.NewAlpha(image.Rect(0, 0, GlyphWidth, GlyphHeight))
	d := &font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: face,
	}

	// Get glyph bounds and advance
	bounds, advance, _ := face.GlyphBounds(r)

	// Calculate horizontal centering offset
	xOffset := fixed.Int26_6((GlyphWidth*64 - advance) / 2)

	// Calculate vertical centering offset
	yOffset := face.Metrics().Ascent +
		fixed.Int26_6(GlyphHeight*64-(face.Metrics().Ascent+face.Metrics().Descent))

	// Set the drawing point
	d.Dot = fixed.Point26_6{
		X: xOffset,
		Y: yOffset,
	}

	d.DrawString(string(r))

	return &GlyphInfo{
		Rune:    r,
		Img:     img,
		Bounds:  bounds,
		Advance: advance,
	}
}

func analyzeFont(ttf *truetype.Font, safe *truetype.Font) []GlyphInfo {
	var glyphs []GlyphInfo
	//safeRunes := getSafeRunes()

	for r := rune(0); r <= 0xFFFF; r++ {
		if ttf.Index(r) != 0 && safe.Index(r) != 0 {
			// Check if the rune is printable
			if !unicode.IsPrint(r) {
				continue
			}
			glyph := renderGlyph(ttf, r)
			glyph.analyzeGlyph()
			// Remove empty glyphs that are not U+0020
			if glyph.Weight == 0 && r != 0x0020 {
				continue
			}
			glyphs = append(glyphs, *glyph)
		}
	}

	return glyphs
}

func debugPrintGlyph(glyph GlyphInfo) {
	fmt.Printf("Unicode: U+%04X\n", glyph.Rune)
	fmt.Printf("Character: %c\n", glyph.Rune)
	fmt.Printf("Weight: %d\n", glyph.Weight)
	fmt.Printf("Row Weights: %v\n", glyph.RowWeights)
	fmt.Sprintf("Hex: 0x%04X\n", uint64(glyph.Bitmap))
	fmt.Printf("Glyph Bitmap:\n%s\n", glyph.Bitmap.String())
	fmt.Println(strings.Repeat("-", 20))
}

func main() {
	font, err := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, err := loadFont("FSD - PragmataProMono.ttf")
	if err != nil {
		panic(err)
	}
	glyphs := analyzeFont(font, safeFont)

	glyphLookup := NewGlyphLookup(glyphs)

	// Test glyphLookup
	for i, glyph := range glyphs {
		bitmapIndex := glyphLookup.BitmapMap[glyph.Bitmap]
		bitmapLookup := glyphLookup.Glyphs[bitmapIndex].Bitmap
		if glyph.Bitmap != bitmapLookup {
			print("Glyph ", i, " does not match bitmap map\n")
			println(glyph.Bitmap.String())
			println(glyphLookup.Glyphs[bitmapLookup].Bitmap.String())
			panic("Bitmap map is incorrect")
		}
	}

	// Test glyphLookup.FindClosestGlyph
	// Loop 100 times, generate a random glyph bitmap, find the closest glyph
	// and compare the weight
	for i := 0; i < 100; i++ {
		// Generate a random glyph bitmap
		block := GlyphBitmap(0x0000000000000000)
		for j := 0; j < 64; j++ {
			if rand.Intn(2) == 1 {
				block |= 1 << uint(j)
			}
		}

		// Find the closest glyph
		closestGlyph := glyphLookup.FindClosestGlyph(block)

		// Show the two bitmaps
		println("Block:")
		println(block.String())
		println("Closest Glyph:")
		println(closestGlyph.Bitmap.String())

		// Compare the weight
		if block.popCount() != closestGlyph.Weight {
			print("Weight does not match\n")
			println(block.String())
			println(closestGlyph.Bitmap.String())
			panic("Weight does not match")
		}
	}

	// Print debug information for each glyph
	for _, glyph := range glyphs {
		debugPrintGlyph(glyph)
	}
	print(len(glyphs), " glyphs analyzed\n")

}
