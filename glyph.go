package img2ansi

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/gob"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"strings"
)

const (
	// GlyphWidth and GlyphHeight define the standard character cell size
	GlyphWidth  = 8
	GlyphHeight = 8
)

// GlyphBitmap represents an 8x8 character as a 64-bit integer.
// Bit layout is row-major with the LSB at the top-left:
// bit (y*GlyphWidth + x) corresponds to pixel (x, y).
// This is the single canonical ordering — all code that touches glyph
// bits must go through Bit/SetBit (see TestGlyphBitmapOrdering).
type GlyphBitmap uint64

// Bit reports whether the pixel at (x, y) is set.
func (g GlyphBitmap) Bit(x, y int) bool {
	if x < 0 || x >= GlyphWidth || y < 0 || y >= GlyphHeight {
		return false
	}
	return g&(1<<(y*GlyphWidth+x)) != 0
}

// SetBit sets or clears the pixel at (x, y).
func (g *GlyphBitmap) SetBit(x, y int, value bool) {
	if x < 0 || x >= GlyphWidth || y < 0 || y >= GlyphHeight {
		return
	}
	pos := y*GlyphWidth + x
	if value {
		*g |= 1 << pos
	} else {
		*g &= ^GlyphBitmap(1 << pos)
	}
}

// String renders the bitmap as 8 rows of █/· for debugging, top row first.
func (g GlyphBitmap) String() string {
	var sb strings.Builder
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			if g.Bit(x, y) {
				sb.WriteRune('█')
			} else {
				sb.WriteRune('·')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// FontBitmaps holds pre-rendered 8x8 character bitmaps for a font.
type FontBitmaps struct {
	glyphs   map[rune]GlyphBitmap
	fallback map[rune]GlyphBitmap
	name     string
}

// FontGlyphData is the serialization format for .glyphs files
// (gzip-compressed gob). The field names are part of the format.
type FontGlyphData struct {
	FontName string
	Glyphs   map[rune]GlyphBitmap
}

// Embedded font glyph data.
//
//   - pxplus_ibm_bios: CC BY-SA 4.0 TTF recreation of the IBM PC BIOS
//     font (CP437 coverage; quadrant blocks synthesized).
//   - font8x8: public domain (dhepper/font8x8); complete block element
//     and box drawing coverage, all 16 quadrants genuine.
//
// To add more fonts:
//  1. Run: ./cmd/compute_glyphs/compute_glyphs -font yourfont.ttf -output fontdata/yourfont.glyphs
//     (or -rom for ROM dumps, -font8x8 for font8x8-style C headers)
//  2. Add: //go:embed fontdata/yourfont.glyphs
//
//go:embed fontdata/pxplus_ibm_bios.glyphs
//go:embed fontdata/font8x8.glyphs
var fontFS embed.FS

// LoadEmbeddedFont loads pre-computed glyph data compiled into the binary,
// e.g. LoadEmbeddedFont("pxplus_ibm_bios").
func LoadEmbeddedFont(name string) (*FontBitmaps, error) {
	data, err := fontFS.ReadFile("fontdata/" + name + ".glyphs")
	if err != nil {
		return nil, fmt.Errorf("no embedded font %q: %w", name, err)
	}
	return decodeGlyphData(data)
}

// LoadGlyphFile loads a .glyphs file (as produced by compute_glyphs)
// from the filesystem.
func LoadGlyphFile(path string) (*FontBitmaps, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read glyph file: %w", err)
	}
	return decodeGlyphData(data)
}

func decodeGlyphData(data []byte) (*FontBitmaps, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	var glyphData FontGlyphData
	if err := gob.NewDecoder(gr).Decode(&glyphData); err != nil {
		return nil, fmt.Errorf("failed to decode glyph data: %w", err)
	}

	fb := &FontBitmaps{
		glyphs:   glyphData.Glyphs,
		fallback: make(map[rune]GlyphBitmap),
		name:     glyphData.FontName,
	}
	fb.synthesizeBlockGlyphs()
	return fb, nil
}

// LoadROMFont parses a classic PC ROM font dump: 256 glyphs of 8 row
// bytes each (2048 bytes total), MSB = leftmost pixel, glyph order =
// CP437 code points. Glyphs are keyed by their Unicode equivalents.
//
// ROM dumps are the most direct source for 8x8 bitmap fonts: the
// bitmaps are correct by construction, with no rasterization geometry
// to get wrong. TTF recreations work too via cmd/compute_glyphs, which
// calibrates and verifies its rasterization (see docs/glyph-research/).
func LoadROMFont(data []byte, name string) (*FontBitmaps, error) {
	const want = 256 * GlyphHeight
	if len(data) != want {
		return nil, fmt.Errorf(
			"ROM font must be %d bytes (256 glyphs x %d rows), got %d",
			want, GlyphHeight, len(data))
	}

	glyphs := make(map[rune]GlyphBitmap, 256)
	for c := 0; c < 256; c++ {
		var g GlyphBitmap
		for y := 0; y < GlyphHeight; y++ {
			row := data[c*GlyphHeight+y]
			for x := 0; x < GlyphWidth; x++ {
				if row&(1<<(7-x)) != 0 {
					g.SetBit(x, y, true)
				}
			}
		}
		glyphs[cp437ToUnicode[c]] = g
	}

	fb := &FontBitmaps{
		glyphs:   glyphs,
		fallback: make(map[rune]GlyphBitmap),
		name:     name,
	}
	fb.synthesizeBlockGlyphs()
	return fb, nil
}

// synthesizeBlockGlyphs fills the fallback map with procedurally built
// bitmaps for the 16 quadrant block characters the dither pipeline can
// emit. These characters have exact geometric definitions, so when a
// font does not provide them — CP437 fonts carry only 6 of the 16, and
// a TTF's missing-glyph box is far worse than no glyph — the correct
// bitmap is constructed. Font-provided glyphs always take precedence.
func (fb *FontBitmaps) synthesizeBlockGlyphs() {
	for _, b := range Blocks {
		if _, ok := fb.glyphs[b.Rune]; ok {
			continue
		}
		if _, ok := fb.fallback[b.Rune]; ok {
			continue
		}
		fb.fallback[b.Rune] = glyphFromQuadrants(b.Quad)
	}
}

// glyphFromQuadrants builds an 8x8 bitmap from a 2x2 quadrant pattern,
// each quadrant covering a 4x4 region of the cell.
func glyphFromQuadrants(q Quadrants) GlyphBitmap {
	var g GlyphBitmap
	quads := [4]bool{q.TopLeft, q.TopRight, q.BottomLeft, q.BottomRight}
	for i, filled := range quads {
		if !filled {
			continue
		}
		baseX := (i % 2) * (GlyphWidth / 2)
		baseY := (i / 2) * (GlyphHeight / 2)
		for y := 0; y < GlyphHeight/2; y++ {
			for x := 0; x < GlyphWidth/2; x++ {
				g.SetBit(baseX+x, baseY+y, true)
			}
		}
	}
	return g
}

// Name returns the font's name.
func (fb *FontBitmaps) Name() string {
	return fb.name
}

// GlyphCount returns the number of glyphs in the primary font.
func (fb *FontBitmaps) GlyphCount() int {
	return len(fb.glyphs)
}

// GetGlyph returns the bitmap for a character, checking fallback if needed.
func (fb *FontBitmaps) GetGlyph(r rune) (GlyphBitmap, bool) {
	if bitmap, exists := fb.glyphs[r]; exists {
		return bitmap, true
	}
	if bitmap, exists := fb.fallback[r]; exists {
		return bitmap, true
	}
	return 0, false
}

// Runes returns all characters available in the primary font.
func (fb *FontBitmaps) Runes() []rune {
	runes := make([]rune, 0, len(fb.glyphs))
	for r := range fb.glyphs {
		runes = append(runes, r)
	}
	return runes
}

// RenderBlocks renders a BlockRune array to an image using the font's
// glyph bitmaps, GlyphWidth*scale x GlyphHeight*scale pixels per cell.
func (fb *FontBitmaps) RenderBlocks(blocks [][]BlockRune, scale int) *image.RGBA {
	if scale < 1 {
		scale = 1
	}

	height := len(blocks)
	if height == 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	width := len(blocks[0])

	cellW, cellH := GlyphWidth*scale, GlyphHeight*scale
	img := image.NewRGBA(image.Rect(0, 0, width*cellW, height*cellH))

	for y, row := range blocks {
		for x, block := range row {
			fb.renderChar(img, block, x*cellW, y*cellH, scale)
		}
	}

	return img
}

// renderChar renders a single character cell at the given pixel position.
func (fb *FontBitmaps) renderChar(img *image.RGBA, block BlockRune, startX, startY, scale int) {
	bitmap, exists := fb.GetGlyph(block.Rune)
	if !exists {
		// Character not in font: render as background
		rect := image.Rect(startX, startY,
			startX+GlyphWidth*scale, startY+GlyphHeight*scale)
		draw.Draw(img, rect, &image.Uniform{rgbToColor(block.BG)}, image.Point{}, draw.Src)
		return
	}

	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			c := rgbToColor(block.BG)
			if bitmap.Bit(x, y) {
				c = rgbToColor(block.FG)
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					img.Set(startX+x*scale+sx, startY+y*scale+sy, c)
				}
			}
		}
	}
}

// rgbToColor converts the package RGB type to a color.Color.
func rgbToColor(rgb RGB) color.Color {
	return color.RGBA{R: rgb.R, G: rgb.G, B: rgb.B, A: 255}
}
