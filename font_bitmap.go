package img2ansi

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/gob"
	"fmt"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"image"
	"image/color"
	"image/draw"
	"io/ioutil"
	"strings"
)

const (
	// GlyphWidth and GlyphHeight define the standard character cell size
	GlyphWidth  = 8
	GlyphHeight = 8
)

// GlyphBitmap represents an 8x8 character as a 64-bit integer
// Each bit represents a pixel: 1 = foreground, 0 = background
type GlyphBitmap uint64

// FontBitmaps holds pre-rendered character bitmaps for a font
type FontBitmaps struct {
	glyphs   map[rune]GlyphBitmap
	fallback map[rune]GlyphBitmap
	name     string
}

// FontGlyphData represents pre-computed glyph bitmaps for a font (for serialization)
type FontGlyphData struct {
	FontName string
	Glyphs   map[rune]GlyphBitmap
}

// Embedded font glyph data
// To add more fonts:
// 1. Run: ./cmd/compute_glyphs/compute_glyphs -font yourfont.ttf -output fontdata/yourfont.glyphs
// 2. Add: //go:embed fontdata/yourfont.glyphs
// 3. The font will be auto-detected when loading fonts/yourfont.ttf
//
//go:embed fontdata/pxplus_ibm_bios.glyphs
var fontFS embed.FS

// getBit checks if a specific bit is set in the bitmap
func (g GlyphBitmap) getBit(x, y int) bool {
	if x < 0 || x >= GlyphWidth || y < 0 || y >= GlyphHeight {
		return false
	}
	return g&(1<<(y*GlyphWidth+x)) != 0
}

// setBit sets a specific bit in the bitmap
func (g *GlyphBitmap) setBit(x, y int, value bool) {
	if x < 0 || x >= GlyphWidth || y < 0 || y >= GlyphHeight {
		return
	}
	pos := y*GlyphWidth + x
	if value {
		*g |= 1 << pos
	} else {
		*g &= ^(1 << pos)
	}
}

// LoadFontBitmaps loads font bitmaps from embedded data or TTF files
func LoadFontBitmaps(primaryPath, fallbackPath string) (*FontBitmaps, error) {
	// First try to load from embedded glyphs
	if strings.HasSuffix(primaryPath, ".ttf") {
		// Try to find corresponding embedded glyph file
		baseName := strings.TrimSuffix(primaryPath, ".ttf")
		if strings.Contains(baseName, "/") {
			parts := strings.Split(baseName, "/")
			baseName = parts[len(parts)-1]
		}
		embeddedName := strings.ToLower(strings.ReplaceAll(baseName, " ", "_")) + ".glyphs"
		embeddedPath := "fontdata/" + embeddedName

		if fb, err := loadEmbeddedGlyphs(embeddedPath); err == nil {
			// Successfully loaded from embedded data
			return fb, nil
		}
	}

	// Fall back to loading from TTF files
	return loadFontBitmapsFromTTF(primaryPath, fallbackPath)
}

// loadEmbeddedGlyphs loads pre-computed glyph data from embedded files
func loadEmbeddedGlyphs(path string) (*FontBitmaps, error) {
	data, err := fontFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded glyphs: %w", err)
	}

	// Create gzip reader
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	// Decode the data
	var glyphData FontGlyphData
	dec := gob.NewDecoder(gr)
	if err := dec.Decode(&glyphData); err != nil {
		return nil, fmt.Errorf("failed to decode glyph data: %w", err)
	}

	// Create FontBitmaps from the loaded data
	fb := &FontBitmaps{
		glyphs:   glyphData.Glyphs,
		fallback: make(map[rune]GlyphBitmap),
		name:     glyphData.FontName,
	}

	return fb, nil
}

// loadFontBitmapsFromTTF pre-renders a TrueType font to bitmaps
func loadFontBitmapsFromTTF(primaryPath, fallbackPath string) (*FontBitmaps, error) {
	fb := &FontBitmaps{
		glyphs:   make(map[rune]GlyphBitmap),
		fallback: make(map[rune]GlyphBitmap),
		name:     primaryPath,
	}

	// Load primary font
	primaryFont, err := loadFont(primaryPath)
	if err != nil {
		return nil, err
	}

	// Load fallback font (optional)
	var fallbackFont *truetype.Font
	if fallbackPath != "" {
		fallbackFont, _ = loadFont(fallbackPath)
	}

	// Pre-render common characters
	// Start with ASCII printable characters
	for r := rune(32); r <= rune(126); r++ {
		fb.glyphs[r] = renderGlyphToBitmap(primaryFont, r)
		if fallbackFont != nil {
			fb.fallback[r] = renderGlyphToBitmap(fallbackFont, r)
		}
	}

	// Add Unicode block characters
	blockChars := []rune{
		' ', '▀', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█',
		'▌', '▍', '▎', '▏', '▐', '░', '▒', '▓',
		'▔', '▕', '▖', '▗', '▘', '▙', '▚', '▛', '▜', '▝', '▞', '▟',
	}
	for _, r := range blockChars {
		fb.glyphs[r] = renderGlyphToBitmap(primaryFont, r)
		if fallbackFont != nil {
			fb.fallback[r] = renderGlyphToBitmap(fallbackFont, r)
		}
	}

	return fb, nil
}

// loadFont loads a TrueType font from file
func loadFont(path string) (*truetype.Font, error) {
	fontBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	font, err := freetype.ParseFont(fontBytes)
	if err != nil {
		return nil, err
	}

	return font, nil
}

// renderGlyphToBitmap renders a single glyph to an 8x8 bitmap
//
// Implementation choices and rationale:
//
//  1. Alpha channel image: We use image.NewAlpha instead of image.NewGray because
//     TrueType rendering produces anti-aliased output. The alpha channel directly
//     represents pixel coverage, giving us the most accurate representation of the
//     glyph shape before thresholding.
//
//  2. 25% threshold (64/255): This low threshold is critical for preserving font
//     details. Anti-aliased text has many edge pixels with 25-75% opacity. A higher
//     threshold (like 50%) would lose these edge pixels, making characters appear
//     broken or too thin. For example, the dot on 'i' or thin serifs might disappear
//     with a 50% threshold but are preserved at 25%.
//
//  3. Dynamic baseline positioning: We calculate the baseline using font metrics
//     (ascent/descent) rather than hardcoding it. This prevents clipping of
//     descenders (g,j,p,q,y) and ensures proper vertical centering for fonts with
//     different proportions.
//
//  4. 8-point font size: We set the font size to 8 points, which works well for
//     fonts designed for terminal use (like IBM BIOS) but may not be optimal for
//     all fonts. This is a limitation we share with the compute_fonts implementation.
func renderGlyphToBitmap(ttfFont *truetype.Font, r rune) GlyphBitmap {
	// Create font face with proper size
	face := truetype.NewFace(ttfFont, &truetype.Options{
		Size:    float64(GlyphHeight), // 8 point size
		DPI:     72,
		Hinting: font.HintingFull,
	})
	defer face.Close()

	// Create an 8x8 alpha image (better for anti-aliasing)
	img := image.NewAlpha(image.Rect(0, 0, GlyphWidth, GlyphHeight))

	// Set up the freetype context
	ctx := freetype.NewContext()
	ctx.SetDPI(72)
	ctx.SetFont(ttfFont)
	ctx.SetFontSize(float64(GlyphHeight))
	ctx.SetClip(img.Bounds())
	ctx.SetDst(img)
	ctx.SetSrc(image.White)
	ctx.SetHinting(font.HintingFull)

	// Get glyph metrics for centering
	// Note: This is simplified - the original uses more complex centering
	metrics := face.Metrics()

	// Calculate baseline position (approximate centering)
	// The original code has complex centering logic, but for now we'll use simple positioning
	ascent := metrics.Ascent >> 6   // Convert from 26.6 fixed point to pixels
	descent := metrics.Descent >> 6 // Descent is typically negative
	baselineY := (GlyphHeight + int(ascent) - int(descent)) / 2

	// Draw the character
	pt := freetype.Pt(0, baselineY)
	ctx.DrawString(string(r), pt)

	// Convert to bitmap using 25% alpha threshold (like compute_fonts)
	var bitmap GlyphBitmap
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if img.AlphaAt(x, y).A > 64 { // 25% threshold
				bitmap.setBit(x, y, true)
			}
		}
	}

	return bitmap
}

// RenderBlocks renders BlockRune array to image using font bitmaps
func (fb *FontBitmaps) RenderBlocks(blocks [][]BlockRune, scale int) *image.RGBA {
	if scale < 1 {
		scale = 1
	}

	height := len(blocks)
	if height == 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	width := len(blocks[0])

	// Create output image
	charSize := GlyphWidth * scale
	img := image.NewRGBA(image.Rect(0, 0, width*charSize, height*charSize))

	// Render each block
	for y, row := range blocks {
		for x, block := range row {
			fb.renderChar(img, block, x*charSize, y*charSize, scale)
		}
	}

	return img
}

// renderChar renders a single character with colors at the specified position
func (fb *FontBitmaps) renderChar(img *image.RGBA, block BlockRune, startX, startY, scale int) {
	// Look up the glyph bitmap
	bitmap, exists := fb.glyphs[block.Rune]
	if !exists {
		// Try fallback font
		bitmap, exists = fb.fallback[block.Rune]
		if !exists {
			// Character not found, render as empty space
			fb.fillRect(img, startX, startY, GlyphWidth*scale, GlyphHeight*scale, rgbToColor(block.BG))
			return
		}
	}

	// Render the bitmap
	fb.renderBitmap(img, bitmap, startX, startY, scale, block.FG, block.BG)
}

// renderBitmap renders a GlyphBitmap at the given position with scaling
func (fb *FontBitmaps) renderBitmap(img *image.RGBA, bitmap GlyphBitmap, startX, startY, scale int, fg, bg RGB) {
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			color := bg
			if bitmap.getBit(x, y) {
				color = fg
			}

			// Apply scaling
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					img.Set(startX+x*scale+sx, startY+y*scale+sy, rgbToColor(color))
				}
			}
		}
	}
}

// fillRect fills a rectangle with the given color
func (fb *FontBitmaps) fillRect(img *image.RGBA, x, y, width, height int, c color.Color) {
	rect := image.Rect(x, y, x+width, y+height)
	draw.Draw(img, rect, &image.Uniform{c}, image.Point{}, draw.Src)
}

// rgbToColor converts our RGB type to color.Color
func rgbToColor(rgb RGB) color.Color {
	return color.RGBA{R: rgb.R, G: rgb.G, B: rgb.B, A: 255}
}

// GetGlyph returns the bitmap for a character, checking fallback if needed
func (fb *FontBitmaps) GetGlyph(r rune) (GlyphBitmap, bool) {
	if bitmap, exists := fb.glyphs[r]; exists {
		return bitmap, true
	}
	if bitmap, exists := fb.fallback[r]; exists {
		return bitmap, true
	}
	return 0, false
}
