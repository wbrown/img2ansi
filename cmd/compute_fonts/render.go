// render.go - Consolidated rendering functionality for img2ansi compute_fonts
//
// This file consolidates all rendering-related code including:
// - ANSI parsing and rendering (from ansi_parser.go)
// - Glyph-based rendering using font bitmaps (from glyph_renderer.go)
// - Block character rendering with patterns (from block_renderer.go)
// - Various rendering strategies (from render_strategy.go)
// - Text-to-image rendering using fonts (from render_output.go)
// - Visual output saving functionality (from save_visual_output.go)
// - Unified ANSI to PNG rendering (from render_to_png_unified.go)
//
// The main entry points are:
// - RenderANSIToPNG(): Renders ANSI files to PNG images
// - RenderTextToImage(): Renders plain text to images using fonts
// - RenderAllToPNG(): Batch renders multiple ANSI files
// - Various helper functions for ANSI parsing and character rendering

package main

import (
	"bufio"
	"fmt"
	"github.com/golang/freetype"
	"github.com/wbrown/img2ansi"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ========== Core Types ==========

// ANSIChar represents a character with its styling
type ANSIChar struct {
	Char      rune
	FgColor   img2ansi.RGB
	BgColor   img2ansi.RGB
	Bold      bool
	Underline bool
}

// BlockPattern defines the pixel pattern for a block character
type BlockPattern uint16 // 4x4 grid = 16 bits

// RenderStrategy defines how to convert image blocks to characters
type RenderStrategy interface {
	RenderBlock(pixels [][]uint8) (rune, error)
	GetBlockSize() (width, height int)
}

// ========== Block Character Patterns ==========

// Common block patterns as 4x4 grids
var BlockPatterns = map[rune]BlockPattern{
	' ':  0x0000, // Empty
	'░':  0x5A5A, // Light shade (checkerboard pattern)
	'▒':  0x55AA, // Medium shade
	'▓':  0xAAFF, // Dark shade
	'█':  0xFFFF, // Full block
	'▀':  0xFF00, // Upper half
	'▄':  0x00FF, // Lower half
	'▌':  0xCCCC, // Left half
	'▐':  0x3333, // Right half
	'▖':  0x00CC, // Lower left quarter
	'▗':  0x0033, // Lower right quarter
	'▘':  0xCC00, // Upper left quarter
	'▙':  0xCCFF, // Upper left + lower half
	'▚':  0xCC33, // Diagonal (upper left + lower right)
	'▛':  0xFFCC, // Upper half + left lower
	'▜':  0xFF33, // Upper half + right lower
	'▝':  0x3300, // Upper right quarter
	'▞':  0x33CC, // Diagonal (upper right + lower left)
	'▟':  0x33FF, // Upper right + lower half
	'▁':  0x000F, // Lower one eighth
	'▂':  0x00FF, // Lower one quarter
	'▃':  0x0FFF, // Lower three eighths
	'▅':  0x3FFF, // Lower five eighths
	'▆':  0x7FFF, // Lower three quarters
	'▇':  0xBFFF, // Lower seven eighths
	'▉':  0xDFFF, // Left seven eighths
	'▊':  0xCFFF, // Left three quarters
	'▋':  0x8FFF, // Left five eighths
	'▍':  0x8CCC, // Left three eighths
	'▎':  0x8888, // Left one quarter
	'▏':  0x8000, // Left one eighth
}

// ========== ANSI Parser ==========

// ANSIParser handles parsing of ANSI escape sequences
type ANSIParser struct {
	// Current state
	currentFg        img2ansi.RGB
	currentBg        img2ansi.RGB
	currentBold      bool
	currentUnderline bool

	// Default colors
	defaultFg img2ansi.RGB
	defaultBg img2ansi.RGB

	// Palette for color lookups
	palette []img2ansi.RGB
}

// Regular expressions for ANSI parsing
var (
	ansiEscapeRe = regexp.MustCompile(`\x1b\[([\d;]+)m`)
	ansiAnyRe    = regexp.MustCompile(`\x1b\[[^m]*m`)
)

// NewANSIParser creates a parser with default settings
func NewANSIParser() *ANSIParser {
	// Ensure palettes are loaded
	loadPaletteOnce()
	
	return &ANSIParser{
		currentFg: img2ansi.RGB{255, 255, 255}, // White
		currentBg: img2ansi.RGB{0, 0, 0},       // Black
		defaultFg: img2ansi.RGB{255, 255, 255},
		defaultBg: img2ansi.RGB{0, 0, 0},
		palette:   nil, // We'll use the global color maps instead
	}
}

// ParseLine parses a line of text with ANSI codes into styled characters
func (p *ANSIParser) ParseLine(line string) []ANSIChar {
	var result []ANSIChar

	// Find all escape sequences
	matches := ansiEscapeRe.FindAllStringSubmatchIndex(line, -1)

	lastEnd := 0
	for _, match := range matches {
		// Add text before this escape sequence
		if match[0] > lastEnd {
			text := line[lastEnd:match[0]]
			for _, r := range text {
				result = append(result, ANSIChar{
					Char:      r,
					FgColor:   p.currentFg,
					BgColor:   p.currentBg,
					Bold:      p.currentBold,
					Underline: p.currentUnderline,
				})
			}
		}

		// Parse the escape sequence
		codes := line[match[2]:match[3]]
		p.parseEscapeCodes(codes)

		lastEnd = match[1]
	}

	// Add remaining text
	if lastEnd < len(line) {
		text := line[lastEnd:]
		for _, r := range text {
			result = append(result, ANSIChar{
				Char:      r,
				FgColor:   p.currentFg,
				BgColor:   p.currentBg,
				Bold:      p.currentBold,
				Underline: p.currentUnderline,
			})
		}
	}

	return result
}

// parseEscapeCodes processes a semicolon-separated list of ANSI codes
func (p *ANSIParser) parseEscapeCodes(codes string) {
	parts := strings.Split(codes, ";")

	for i := 0; i < len(parts); i++ {
		code, err := strconv.Atoi(parts[i])
		if err != nil {
			continue
		}

		switch code {
		case 0: // Reset
			p.currentFg = p.defaultFg
			p.currentBg = p.defaultBg
			p.currentBold = false
			p.currentUnderline = false

		case 1: // Bold
			p.currentBold = true

		case 4: // Underline
			p.currentUnderline = true

		case 22: // Normal intensity
			p.currentBold = false

		case 24: // No underline
			p.currentUnderline = false

		// Foreground colors 30-37
		case 30, 31, 32, 33, 34, 35, 36, 37:
			p.currentFg = ansi256ToRGBForParsing(code - 30)

		// Foreground colors 90-97 (bright)
		case 90, 91, 92, 93, 94, 95, 96, 97:
			p.currentFg = ansi256ToRGBForParsing(8 + code - 90)

		// Background colors 40-47
		case 40, 41, 42, 43, 44, 45, 46, 47:
			p.currentBg = ansi256ToRGBForParsing(code - 40)

		// Background colors 100-107 (bright)
		case 100, 101, 102, 103, 104, 105, 106, 107:
			p.currentBg = ansi256ToRGBForParsing(8 + code - 100)

		case 38: // Extended foreground color
			if i+2 < len(parts) && parts[i+1] == "5" {
				// 256-color mode
				if colorCode, err := strconv.Atoi(parts[i+2]); err == nil && colorCode < 256 {
					p.currentFg = ansi256ToRGBForParsing(colorCode)
				}
				i += 2
			}

		case 48: // Extended background color
			if i+2 < len(parts) && parts[i+1] == "5" {
				// 256-color mode
				if colorCode, err := strconv.Atoi(parts[i+2]); err == nil && colorCode < 256 {
					p.currentBg = ansi256ToRGBForParsing(colorCode)
				}
				i += 2
			}

		case 39: // Default foreground
			p.currentFg = p.defaultFg

		case 49: // Default background
			p.currentBg = p.defaultBg
		}
	}
}

// ========== Glyph Renderer ==========

// GlyphRenderer provides a unified way to render characters using Glyph bitmaps
type GlyphRenderer struct {
	lookup *GlyphLookup
	scale  int // scaling factor for rendering (1 = 8x8, 2 = 16x16, etc.)
}

// NewGlyphRenderer creates a new renderer with the given glyph lookup table
func NewGlyphRenderer(lookup *GlyphLookup, scale int) *GlyphRenderer {
	if scale <= 0 {
		scale = 1
	}
	return &GlyphRenderer{
		lookup: lookup,
		scale:  scale,
	}
}

// RenderCharacter renders a single character at the given position using its bitmap
func (gr *GlyphRenderer) RenderCharacter(img *image.RGBA, char rune, x, y int, fg, bg img2ansi.RGB) {
	// First check if it's a block character that we can render directly
	if IsBlockCharacter(char) {
		gr.renderBlockCharacter(img, char, x, y, fg, bg)
		return
	}

	// Otherwise look up the glyph bitmap
	glyphInfo := gr.lookup.LookupRune(char)
	if glyphInfo == nil {
		// Character not found, render as empty space
		gr.fillRect(img, x, y, 8*gr.scale, 8*gr.scale, rgbToColor(bg))
		return
	}

	// Render the glyph bitmap
	gr.renderGlyphBitmap(img, glyphInfo.Bitmap, x, y, fg, bg)
}

// renderGlyphBitmap renders a GlyphBitmap at the given position
func (gr *GlyphRenderer) renderGlyphBitmap(img *image.RGBA, bitmap GlyphBitmap, startX, startY int, fg, bg img2ansi.RGB) {
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < GlyphWidth; x++ {
			c := bg
			if getBit(bitmap, x, y) {
				c = fg
			}

			// Apply scaling
			for sy := 0; sy < gr.scale; sy++ {
				for sx := 0; sx < gr.scale; sx++ {
					img.Set(startX+x*gr.scale+sx, startY+y*gr.scale+sy, rgbToColor(c))
				}
			}
		}
	}
}

// renderBlockCharacter handles Unicode block characters with hardcoded patterns
func (gr *GlyphRenderer) renderBlockCharacter(img *image.RGBA, char rune, startX, startY int, fg, bg img2ansi.RGB) {
	// Convert block characters to bitmaps
	bitmap := blockCharToBitmap(char)
	gr.renderGlyphBitmap(img, bitmap, startX, startY, fg, bg)
}

// fillRect fills a rectangle with the given color
func (gr *GlyphRenderer) fillRect(img *image.RGBA, x, y, width, height int, c color.Color) {
	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			img.Set(x+dx, y+dy, c)
		}
	}
}

// RenderANSIFile renders an ANSI file to PNG using glyph bitmaps
func (gr *GlyphRenderer) RenderANSIFile(ansiFile, outputFile string) error {
	file, err := os.Open(ansiFile)
	if err != nil {
		return fmt.Errorf("error opening %s: %v", ansiFile, err)
	}
	defer file.Close()

	// Parse ANSI file
	chars, err := parseANSIFile(file)
	if err != nil {
		return fmt.Errorf("error parsing ANSI: %v", err)
	}

	if len(chars) == 0 {
		return fmt.Errorf("no characters found in ANSI file")
	}

	// Create image
	height := len(chars)
	width := len(chars[0])
	img := image.NewRGBA(image.Rect(0, 0, width*8*gr.scale, height*8*gr.scale))

	// Render each character
	for y, row := range chars {
		for x, char := range row {
			gr.RenderCharacter(img, char.Char, x*8*gr.scale, y*8*gr.scale, char.FgColor, char.BgColor)
		}
	}

	// Save PNG
	out, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer out.Close()

	return png.Encode(out, img)
}

// ========== Block Renderer ==========

// BlockRenderer provides unified block character rendering
type BlockRenderer struct {
	scale    int
	fgColor  img2ansi.RGB
	bgColor  img2ansi.RGB
	patterns map[rune]BlockPattern
}

// NewBlockRenderer creates a renderer with default settings
func NewBlockRenderer() *BlockRenderer {
	return &BlockRenderer{
		scale:    1,
		fgColor:  img2ansi.RGB{255, 255, 255}, // White
		bgColor:  img2ansi.RGB{0, 0, 0},       // Black
		patterns: BlockPatterns,
	}
}

// SetScale sets the rendering scale (1 = 8x8, 2 = 16x16, etc.)
func (r *BlockRenderer) SetScale(scale int) {
	if scale < 1 {
		scale = 1
	}
	r.scale = scale
}

// SetColors sets foreground and background colors
func (r *BlockRenderer) SetColors(fg, bg img2ansi.RGB) {
	r.fgColor = fg
	r.bgColor = bg
}

// RenderCharacter renders a block character to an image at the specified position
func (r *BlockRenderer) RenderCharacter(img *image.RGBA, char rune, x, y int) {
	pattern, exists := r.patterns[char]
	if !exists {
		// Unknown character - render as full block
		pattern = 0xFFFF
	}

	// Base size is 8x8, scaled by r.scale
	size := 8 * r.scale

	// Render 4x4 pattern scaled to target size
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			// Map to 4x4 grid
			gridY := py * 4 / size
			gridX := px * 4 / size
			bitPos := gridY*4 + gridX

			// Check if bit is set in pattern
			var pixelColor img2ansi.RGB
			if (pattern & (1 << (15 - bitPos))) != 0 {
				pixelColor = r.fgColor
			} else {
				pixelColor = r.bgColor
			}

			// Set pixel in image
			imgX := x + px
			imgY := y + py
			if imgX >= 0 && imgX < img.Bounds().Dx() && imgY >= 0 && imgY < img.Bounds().Dy() {
				img.Set(imgX, imgY, rgbToColor(pixelColor))
			}
		}
	}
}

// RenderCharacterBitmap renders a character using a GlyphBitmap (8x8)
func (r *BlockRenderer) RenderCharacterBitmap(img *image.RGBA, bitmap GlyphBitmap, x, y int) {
	size := 8 * r.scale

	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			// Map to 8x8 grid
			gridY := py * 8 / size
			gridX := px * 8 / size

			// Check if bit is set in bitmap
			var pixelColor img2ansi.RGB
			if getBit(bitmap, gridX, gridY) {
				pixelColor = r.fgColor
			} else {
				pixelColor = r.bgColor
			}

			// Set pixel in image
			imgX := x + px
			imgY := y + py
			if imgX >= 0 && imgX < img.Bounds().Dx() && imgY >= 0 && imgY < img.Bounds().Dy() {
				img.Set(imgX, imgY, rgbToColor(pixelColor))
			}
		}
	}
}

// HasPattern returns true if the character has a defined pattern
func (r *BlockRenderer) HasPattern(char rune) bool {
	_, exists := r.patterns[char]
	return exists
}

// ========== Render Strategies ==========

// HalfBlockStrategy uses half-block characters (▀ ▄ ▌ ▐)
type HalfBlockStrategy struct {
	patterns map[string]rune
}

func NewHalfBlockStrategy() *HalfBlockStrategy {
	return &HalfBlockStrategy{
		patterns: map[string]rune{
			"00": ' ', // Empty
			"11": '█', // Full block
			"10": '▀', // Upper half
			"01": '▄', // Lower half
		},
	}
}

func (h *HalfBlockStrategy) GetBlockSize() (int, int) {
	return 1, 2 // 1 wide, 2 tall
}

func (h *HalfBlockStrategy) RenderBlock(pixels [][]uint8) (rune, error) {
	if len(pixels) != 2 || len(pixels[0]) != 1 {
		return ' ', fmt.Errorf("expected 1x2 block, got %dx%d", len(pixels[0]), len(pixels))
	}

	// Create pattern key from top and bottom pixels
	top := "0"
	if pixels[0][0] > 128 {
		top = "1"
	}
	bottom := "0"
	if pixels[1][0] > 128 {
		bottom = "1"
	}

	pattern := top + bottom
	if char, ok := h.patterns[pattern]; ok {
		return char, nil
	}

	return ' ', nil
}

// ShadingStrategy uses shading characters (░ ▒ ▓ █)
type ShadingStrategy struct {
	thresholds []uint8
	chars      []rune
}

func NewShadingStrategy() *ShadingStrategy {
	return &ShadingStrategy{
		thresholds: []uint8{64, 128, 192, 224}, // 25%, 50%, 75%, 87.5%
		chars:      []rune{' ', '░', '▒', '▓', '█'},
	}
}

func (s *ShadingStrategy) GetBlockSize() (int, int) {
	return 2, 2 // 2x2 block
}

func (s *ShadingStrategy) RenderBlock(pixels [][]uint8) (rune, error) {
	if len(pixels) != 2 || len(pixels[0]) != 2 {
		return ' ', fmt.Errorf("expected 2x2 block")
	}

	// Calculate average brightness
	total := 0
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			total += int(pixels[y][x])
		}
	}
	avg := uint8(total / 4)

	// Find appropriate shading character
	for i, threshold := range s.thresholds {
		if avg < threshold {
			return s.chars[i], nil
		}
	}

	return s.chars[len(s.chars)-1], nil
}

// GlyphMatchingStrategy uses the existing glyph matching system
type GlyphMatchingStrategy struct {
	lookup *GlyphLookup
}

func NewGlyphMatchingStrategy(lookup *GlyphLookup) *GlyphMatchingStrategy {
	return &GlyphMatchingStrategy{lookup: lookup}
}

func (g *GlyphMatchingStrategy) GetBlockSize() (int, int) {
	return 8, 8 // Full character size
}

func (g *GlyphMatchingStrategy) RenderBlock(pixels [][]uint8) (rune, error) {
	if len(pixels) != 8 || len(pixels[0]) != 8 {
		return ' ', fmt.Errorf("expected 8x8 block")
	}

	// Convert to GlyphBitmap
	var bitmap GlyphBitmap
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if pixels[y][x] > 128 {
				bitmap |= 1 << (y*8 + x)
			}
		}
	}

	match := g.lookup.FindClosestGlyph(bitmap)
	return match.Rune, nil
}

// SelectRenderStrategy chooses the best strategy based on font capabilities
func SelectRenderStrategy(caps *FontCapabilities, lookup *GlyphLookup) RenderStrategy {
	// If we have quarter blocks, use full 2x2 blocks (not applicable for IBM BIOS)
	if caps.HasBlocks == QuarterBlocks || caps.HasBlocks == FullBlocks {
		// Would return QuarterBlockStrategy here
		return NewGlyphMatchingStrategy(lookup)
	}

	// If we have half blocks, we can use them for simple vertical patterns
	if caps.HasBlocks == HalfBlocks {
		// For now, prefer glyph matching for better quality
		// Could use half blocks for specific cases
		return NewGlyphMatchingStrategy(lookup)
	}

	// If we only have shading, use shading strategy
	if caps.HasShading {
		return NewShadingStrategy()
	}

	// Fall back to full glyph matching
	return NewGlyphMatchingStrategy(lookup)
}

// ========== Utility Functions ==========

// IsBlockCharacter returns true if the character is a known block character
func IsBlockCharacter(char rune) bool {
	_, exists := BlockPatterns[char]
	return exists
}

// GetBlockDensity returns the density (0.0-1.0) of a block character
func GetBlockDensity(char rune) float64 {
	pattern, exists := BlockPatterns[char]
	if !exists {
		return 0.0
	}

	// Count set bits
	count := 0
	for i := 0; i < 16; i++ {
		if (pattern & (1 << i)) != 0 {
			count++
		}
	}

	return float64(count) / 16.0
}

// blockCharToBitmap converts Unicode block characters to GlyphBitmap representation
func blockCharToBitmap(char rune) GlyphBitmap {
	var bitmap GlyphBitmap

	switch char {
	case ' ':
		// All zeros (empty)
		bitmap = 0

	case '█':
		// All ones (full block)
		bitmap = ^GlyphBitmap(0)

	case '▀':
		// Upper half
		for y := 0; y < 4; y++ {
			for x := 0; x < 8; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▄':
		// Lower half
		for y := 4; y < 8; y++ {
			for x := 0; x < 8; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▌':
		// Left half
		for y := 0; y < 8; y++ {
			for x := 0; x < 4; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▐':
		// Right half
		for y := 0; y < 8; y++ {
			for x := 4; x < 8; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '░':
		// Light shade (25% - sparse dot pattern)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if (x+y)%4 == 0 {
					bitmap |= 1 << (y*8 + x)
				}
			}
		}

	case '▒':
		// Medium shade (50% - checkerboard)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if (x+y)%2 == 0 {
					bitmap |= 1 << (y*8 + x)
				}
			}
		}

	case '▓':
		// Dark shade (75% - dense pattern)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if (x+y)%4 != 0 {
					bitmap |= 1 << (y*8 + x)
				}
			}
		}

	case '▘':
		// Quadrant upper left
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▝':
		// Quadrant upper right
		for y := 0; y < 4; y++ {
			for x := 4; x < 8; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▖':
		// Quadrant lower left
		for y := 4; y < 8; y++ {
			for x := 0; x < 4; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▗':
		// Quadrant lower right
		for y := 4; y < 8; y++ {
			for x := 4; x < 8; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▚':
		// Diagonal upper left + lower right
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}
		for y := 4; y < 8; y++ {
			for x := 4; x < 8; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▞':
		// Diagonal upper right + lower left
		for y := 0; y < 4; y++ {
			for x := 4; x < 8; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}
		for y := 4; y < 8; y++ {
			for x := 0; x < 4; x++ {
				bitmap |= 1 << (y*8 + x)
			}
		}

	case '▛':
		// Three quarters: missing lower right
		bitmap = ^GlyphBitmap(0)
		for y := 4; y < 8; y++ {
			for x := 4; x < 8; x++ {
				bitmap &= ^(1 << (y*8 + x))
			}
		}

	case '▜':
		// Three quarters: missing lower left
		bitmap = ^GlyphBitmap(0)
		for y := 4; y < 8; y++ {
			for x := 0; x < 4; x++ {
				bitmap &= ^(1 << (y*8 + x))
			}
		}

	case '▙':
		// Three quarters: missing upper right
		bitmap = ^GlyphBitmap(0)
		for y := 0; y < 4; y++ {
			for x := 4; x < 8; x++ {
				bitmap &= ^(1 << (y*8 + x))
			}
		}

	case '▟':
		// Three quarters: missing upper left
		bitmap = ^GlyphBitmap(0)
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				bitmap &= ^(1 << (y*8 + x))
			}
		}
	}

	return bitmap
}

// StripANSICodes removes all ANSI escape sequences from a string
func StripANSICodes(s string) string {
	return ansiAnyRe.ReplaceAllString(s, "")
}

// CountVisibleChars counts non-ANSI characters in a string
func CountVisibleChars(s string) int {
	stripped := StripANSICodes(s)
	return len([]rune(stripped))
}

// parseANSIFile parses an ANSI file into a 2D array of characters with colors
func parseANSIFile(file *os.File) ([][]ANSIChar, error) {
	var result [][]ANSIChar

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		chars := parseANSILine(line)
		if len(chars) > 0 {
			result = append(result, chars)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Ensure all rows have the same length
	if len(result) > 0 {
		maxWidth := 0
		for _, row := range result {
			if len(row) > maxWidth {
				maxWidth = len(row)
			}
		}

		// Pad shorter rows
		for i := range result {
			for len(result[i]) < maxWidth {
				result[i] = append(result[i], ANSIChar{
					Char:    ' ',
					FgColor: img2ansi.RGB{255, 255, 255},
					BgColor: img2ansi.RGB{0, 0, 0},
				})
			}
		}
	}

	return result, nil
}

// Global palette tables loaded once
var (
	ansi256ColorMap  map[int]img2ansi.RGB
	ansi16ColorMap   map[int]img2ansi.RGB
	paletteLoaded    bool
)

// loadPaletteOnce ensures the ANSI palettes are loaded
func loadPaletteOnce() {
	if !paletteLoaded {
		// Load 256 color palette
		fgTable256, _, err := img2ansi.LoadPalette("ansi256")
		if err != nil {
			fmt.Printf("Warning: Failed to load ansi256 palette: %v\n", err)
		} else {
			// Build color map indexed by ANSI code from AnsiData
			ansi256ColorMap = make(map[int]img2ansi.RGB)
			for _, entry := range fgTable256.AnsiData {
				// Parse the ANSI code from the Value field
				// Value is like "38;5;0" for foreground
				var colorCode int
				if _, err := fmt.Sscanf(entry.Value, "38;5;%d", &colorCode); err == nil {
					// Convert uint32 key to RGB
					ansi256ColorMap[colorCode] = img2ansi.RGB{
						R: uint8((entry.Key >> 16) & 0xFF),
						G: uint8((entry.Key >> 8) & 0xFF),
						B: uint8(entry.Key & 0xFF),
					}
				}
			}
		}
		
		// Load 16 color palette  
		fgTable16, _, err := img2ansi.LoadPalette("ansi16")
		if err != nil {
			fmt.Printf("Warning: Failed to load ansi16 palette: %v\n", err)
		} else {
			// Build color map indexed by ANSI code from AnsiData
			ansi16ColorMap = make(map[int]img2ansi.RGB)
			for _, entry := range fgTable16.AnsiData {
				// Parse the basic ANSI code
				// For 16 colors, Value is like "30" to "37" and "90" to "97"
				var colorCode int
				fmt.Sscanf(entry.Value, "%d", &colorCode)
				
				// Map to 0-15 range
				if colorCode >= 30 && colorCode <= 37 {
					colorCode = colorCode - 30
				} else if colorCode >= 90 && colorCode <= 97 {
					colorCode = colorCode - 90 + 8
				} else if colorCode >= 40 && colorCode <= 47 {
					colorCode = colorCode - 40
				} else if colorCode >= 100 && colorCode <= 107 {
					colorCode = colorCode - 100 + 8
				}
				
				// Convert uint32 key to RGB
				ansi16ColorMap[colorCode] = img2ansi.RGB{
					R: uint8((entry.Key >> 16) & 0xFF),
					G: uint8((entry.Key >> 8) & 0xFF),
					B: uint8(entry.Key & 0xFF),
				}
			}
		}
		
		paletteLoaded = true
	}
}

// ansi256ToRGBForParsing converts ANSI color codes to RGB using the actual palette
func ansi256ToRGBForParsing(code int) img2ansi.RGB {
	loadPaletteOnce()
	
	// First check 256-color map (it includes codes 0-15 with proper values)
	if ansi256ColorMap != nil {
		if color, ok := ansi256ColorMap[code]; ok {
			return color
		}
	}
	
	// Fall back to 16-color map only if 256-color map doesn't have it
	if code < 16 && ansi16ColorMap != nil {
		if color, ok := ansi16ColorMap[code]; ok {
			return color
		}
	}
	
	// Fallback to basic colors if palette loading failed or color not found
	if code < 16 {
		basic16 := []img2ansi.RGB{
			{0, 0, 0}, {170, 0, 0}, {0, 170, 0}, {170, 85, 0},
			{0, 0, 170}, {170, 0, 170}, {0, 170, 170}, {170, 170, 170},
			{85, 85, 85}, {255, 85, 85}, {85, 255, 85}, {255, 255, 85},
			{85, 85, 255}, {255, 85, 255}, {85, 255, 255}, {255, 255, 255},
		}
		return basic16[code]
	}
	
	// For codes 16-255, use the standard ANSI 256 color formula if not in map
	if code >= 16 && code <= 231 {
		// 216 color cube (codes 16-231)
		code -= 16
		r := (code / 36) * 51
		g := ((code % 36) / 6) * 51
		b := (code % 6) * 51
		return img2ansi.RGB{uint8(r), uint8(g), uint8(b)}
	} else if code >= 232 && code <= 255 {
		// Grayscale (codes 232-255)
		gray := 8 + (code-232)*10
		return img2ansi.RGB{uint8(gray), uint8(gray), uint8(gray)}
	}
	
	// Default fallback
	return img2ansi.RGB{128, 128, 128}
}

// parseANSILine parses a single line of ANSI text
func parseANSILine(line string) []ANSIChar {
	var chars []ANSIChar
	currentFG := img2ansi.RGB{255, 255, 255} // Default white
	currentBG := img2ansi.RGB{0, 0, 0}       // Default black

	i := 0
	for i < len(line) {
		if i+1 < len(line) && line[i] == '\x1b' && line[i+1] == '[' {
			// Parse ANSI escape sequence
			i += 2
			start := i
			for i < len(line) && line[i] != 'm' {
				i++
			}
			if i < len(line) {
				codes := strings.Split(line[start:i], ";")
				for j := 0; j < len(codes); j++ {
					code := codes[j]
					var codeNum int
					fmt.Sscanf(code, "%d", &codeNum)
					
					if code == "38" && j+2 < len(codes) && codes[j+1] == "5" {
						// 256-color foreground
						var colorCode int
						fmt.Sscanf(codes[j+2], "%d", &colorCode)
						currentFG = ansi256ToRGBForParsing(colorCode)
						j += 2 // Skip the "5" and color code
					} else if code == "48" && j+2 < len(codes) && codes[j+1] == "5" {
						// 256-color background
						var colorCode int
						fmt.Sscanf(codes[j+2], "%d", &colorCode)
						currentBG = ansi256ToRGBForParsing(colorCode)
						j += 2 // Skip the "5" and color code
					} else if code == "0" {
						// Reset
						currentFG = img2ansi.RGB{255, 255, 255}
						currentBG = img2ansi.RGB{0, 0, 0}
					} else if codeNum >= 30 && codeNum <= 37 {
						// Basic foreground colors (30-37)
						currentFG = ansi256ToRGBForParsing(codeNum - 30)
					} else if codeNum >= 40 && codeNum <= 47 {
						// Basic background colors (40-47)
						currentBG = ansi256ToRGBForParsing(codeNum - 40)
					} else if codeNum >= 90 && codeNum <= 97 {
						// Bright foreground colors (90-97)
						currentFG = ansi256ToRGBForParsing(codeNum - 90 + 8)
					} else if codeNum >= 100 && codeNum <= 107 {
						// Bright background colors (100-107)
						currentBG = ansi256ToRGBForParsing(codeNum - 100 + 8)
					}
				}
				i++ // Skip 'm'
			}
		} else if i+2 < len(line) && line[i] == 0xe2 && line[i+1] == 0x96 {
			// UTF-8 encoded Unicode block character
			ch := rune(0x2580 + int(line[i+2]) - 0x80)
			chars = append(chars, ANSIChar{
				Char:    ch,
				FgColor: currentFG,
				BgColor: currentBG,
			})
			i += 3
		} else {
			// Regular character
			chars = append(chars, ANSIChar{
				Char:    rune(line[i]),
				FgColor: currentFG,
				BgColor: currentBG,
			})
			i++
		}
	}

	return chars
}

// colorsEqual compares two colors for equality
func colorsEqual(c1, c2 color.Color) bool {
	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

// rgbsEqual compares two RGB values for equality
func rgbsEqual(c1, c2 img2ansi.RGB) bool {
	return c1 == c2
}

// ========== High-Level Rendering Functions ==========

// RenderANSIToPNG is the main function to render ANSI files to PNG using GlyphRenderer
func RenderANSIToPNG(ansiFile string, outputFile string) error {
	// Load fonts and create glyph lookup
	font, _ := loadFont("PxPlus_IBM_BIOS.ttf")
	safeFont, _ := loadFont("FSD - PragmataProMono.ttf")
	glyphs := analyzeFont(font, safeFont)
	lookup := NewGlyphLookup(glyphs)

	// Create renderer with scale 1 (8x8 pixels per character)
	renderer := NewGlyphRenderer(lookup, 1)

	// Render the file
	return renderer.RenderANSIFile(ansiFile, outputFile)
}

// RenderANSIToPNGUnified is an alias for RenderANSIToPNG (keeping for compatibility)
func RenderANSIToPNGUnified(ansiFile string, outputFile string) error {
	return RenderANSIToPNG(ansiFile, outputFile)
}

// RenderAndSave renders ANSI to PNG with standard error handling
func RenderAndSave(ansFile, pngFile string) error {
	fmt.Printf("Rendering to PNG...\n")
	if err := RenderANSIToPNG(ansFile, pngFile); err != nil {
		return fmt.Errorf("error rendering to PNG: %v", err)
	}
	fmt.Printf("Saved: %s\n", pngFile)
	return nil
}

// RenderTextToImage renders text output back to an image using the font
func RenderTextToImage(textFile string, fontPath string, outputFile string) error {
	// Read the text file
	content, err := os.ReadFile(textFile)
	if err != nil {
		return fmt.Errorf("error reading text file: %v", err)
	}

	// Load the font
	fontBytes, err := os.ReadFile(fontPath)
	if err != nil {
		return fmt.Errorf("error reading font: %v", err)
	}

	f, err := freetype.ParseFont(fontBytes)
	if err != nil {
		return fmt.Errorf("error parsing font: %v", err)
	}

	// Split into lines
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 {
		return fmt.Errorf("no content in file")
	}

	// Set up font rendering
	fontSize := 8.0
	dpi := 72.0

	ctx := freetype.NewContext()
	ctx.SetDPI(dpi)
	ctx.SetFont(f)
	ctx.SetFontSize(fontSize)

	// Calculate image dimensions
	// Each character is 8x8 pixels
	charWidth := 8
	charHeight := 8

	// Find the longest line
	maxLineLength := 0
	for _, line := range lines {
		if len(line) > maxLineLength {
			maxLineLength = len(line)
		}
	}

	// Create image
	imgWidth := maxLineLength * charWidth
	imgHeight := len(lines) * charHeight

	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)

	ctx.SetClip(img.Bounds())
	ctx.SetDst(img)
	ctx.SetSrc(&image.Uniform{color.White})

	// Render each line
	for y, line := range lines {
		if line == "" {
			continue
		}

		// Render each character
		for x, ch := range line {
			// Position for this character
			pt := freetype.Pt(x*charWidth, y*charHeight+charHeight-1)
			ctx.DrawString(string(ch), pt)
		}
	}

	// Save the image
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output file: %v", err)
	}
	defer outFile.Close()

	err = png.Encode(outFile, img)
	if err != nil {
		return fmt.Errorf("error encoding PNG: %v", err)
	}

	return nil
}

// ========== Batch Rendering Functions ==========

// RenderAllToPNG renders all .ans files to PNG
func RenderAllToPNG() {
	// Get all .ans files in the output directory
	ansDir := "outputs/ans"
	pngDir := "outputs/png"
	
	// Ensure PNG directory exists
	os.MkdirAll(pngDir, 0755)
	
	// Read directory
	entries, err := os.ReadDir(ansDir)
	if err != nil {
		fmt.Printf("Error reading directory %s: %v\n", ansDir, err)
		return
	}
	
	// Process each .ans file
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ans") {
			ansFile := fmt.Sprintf("%s/%s", ansDir, entry.Name())
			pngFile := fmt.Sprintf("%s/%s", pngDir, strings.TrimSuffix(entry.Name(), ".ans")+".png")
			
			fmt.Printf("Rendering %s to %s...\n", ansFile, pngFile)
			if err := RenderANSIToPNG(ansFile, pngFile); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}
	}
}

// RenderAllToPNGUnified is an alias for RenderAllToPNG (keeping for compatibility)
func RenderAllToPNGUnified() {
	RenderAllToPNG()
}

// RenderAllOutputs renders both threshold and Atkinson outputs
func RenderAllOutputs() {
	fmt.Println("\nRendering character outputs to PNG using IBM BIOS font...")

	// Render threshold output
	err := RenderTextToImage(
		"baboon_threshold_output.txt",
		"PxPlus_IBM_BIOS.ttf",
		"baboon_threshold_rendered.png",
	)
	if err != nil {
		fmt.Printf("Error rendering threshold output: %v\n", err)
	} else {
		fmt.Println("Created: baboon_threshold_rendered.png")
	}

	// Render Atkinson output
	err = RenderTextToImage(
		"baboon_atkinson_output.txt",
		"PxPlus_IBM_BIOS.ttf",
		"baboon_atkinson_rendered.png",
	)
	if err != nil {
		fmt.Printf("Error rendering Atkinson output: %v\n", err)
	} else {
		fmt.Println("Created: baboon_atkinson_rendered.png")
	}

	// Also create a side-by-side comparison
	createSideBySideComparison()
}

// createSideBySideComparison creates a comparison image
func createSideBySideComparison() error {
	// Load the rendered images
	thresholdFile, err := os.Open("baboon_threshold_rendered.png")
	if err != nil {
		return err
	}
	defer thresholdFile.Close()

	thresholdImg, _, err := image.Decode(thresholdFile)
	if err != nil {
		return err
	}

	atkinsonFile, err := os.Open("baboon_atkinson_rendered.png")
	if err != nil {
		return err
	}
	defer atkinsonFile.Close()

	atkinsonImg, _, err := image.Decode(atkinsonFile)
	if err != nil {
		return err
	}

	// Create side-by-side image
	bounds1 := thresholdImg.Bounds()
	bounds2 := atkinsonImg.Bounds()

	width := bounds1.Dx() + bounds2.Dx() + 20 // 20 pixel gap
	height := bounds1.Dy()
	if bounds2.Dy() > height {
		height = bounds2.Dy()
	}

	combined := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with gray background
	draw.Draw(combined, combined.Bounds(), &image.Uniform{color.Gray{128}}, image.Point{}, draw.Src)

	// Draw threshold on left
	draw.Draw(combined, bounds1, thresholdImg, bounds1.Min, draw.Src)

	// Draw Atkinson on right
	offset := image.Pt(bounds1.Dx()+20, 0)
	draw.Draw(combined, bounds2.Add(offset), atkinsonImg, bounds2.Min, draw.Src)

	// Save combined image
	outFile, err := os.Create(OutputPath("baboon_comparison.png"))
	if err != nil {
		return err
	}
	defer outFile.Close()

	err = png.Encode(outFile, combined)
	if err != nil {
		return err
	}

	fmt.Println("Created: baboon_comparison.png (side-by-side comparison)")
	return nil
}

// ========== Special Purpose Functions ==========

// LoadANSIFile loads and parses an ANSI file
func LoadANSIFile(filename string) ([][]ANSIChar, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return ParseANSIReader(file)
}

// ParseANSIReader parses ANSI from any reader
func ParseANSIReader(r io.Reader) ([][]ANSIChar, error) {
	parser := NewANSIParser()
	scanner := bufio.NewScanner(r)

	var result [][]ANSIChar
	for scanner.Scan() {
		line := scanner.Text()
		chars := parser.ParseLine(line)
		result = append(result, chars)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// RenderANSIToText converts parsed ANSI back to text with escape codes
func RenderANSIToText(chars [][]ANSIChar, w io.Writer) error {
	var lastFg, lastBg img2ansi.RGB

	for _, line := range chars {
		for _, ch := range line {
			// Only output color changes when needed
			if ch.FgColor != lastFg || ch.BgColor != lastBg {
				fgIdx := -1
				bgIdx := -1

				// Load 16-color palette from main package
				fgTable, _, err := img2ansi.LoadPalette("ansi16")
				if err == nil {
					// Extract RGB colors and find indices
					for i, entry := range fgTable.AnsiData {
						pColor := img2ansi.RGB{
							R: uint8((entry.Key >> 16) & 0xFF),
							G: uint8((entry.Key >> 8) & 0xFF),
							B: uint8(entry.Key & 0xFF),
						}
						if rgbsEqual(pColor, ch.FgColor) {
							fgIdx = i
						}
						if rgbsEqual(pColor, ch.BgColor) {
							bgIdx = i
						}
					}
				}

				// Output color codes
				if fgIdx >= 0 && bgIdx >= 0 {
					// Use 16-color codes
					fgCode := GetAnsi16FgCode(fgIdx)
					bgCode := GetAnsi16BgCode(bgIdx)
					fmt.Fprintf(w, "\x1b[%s;%sm", fgCode, bgCode)
				} else {
					// Use 24-bit RGB codes as fallback
					fmt.Fprintf(w, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", 
						ch.FgColor.R, ch.FgColor.G, ch.FgColor.B,
						ch.BgColor.R, ch.BgColor.G, ch.BgColor.B)
				}

				lastFg = ch.FgColor
				lastBg = ch.BgColor
			}

			// Output the character
			fmt.Fprintf(w, "%c", ch.Char)
		}
		fmt.Fprintln(w)
	}

	// Reset at end
	fmt.Fprintln(w, "\x1b[0m")
	return nil
}

// SaveVisualOutput saves the character representation to a text file
func SaveVisualOutput(blocks [][]GlyphBitmap, lookup *GlyphLookup, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Build the output
	var output strings.Builder

	for _, row := range blocks {
		for _, block := range row {
			match := lookup.FindClosestGlyph(block)
			output.WriteRune(match.Rune)
		}
		output.WriteString("\n")
	}

	_, err = file.WriteString(output.String())
	return err
}

// SaveComparisonHTML creates an HTML file showing the images side by side
func SaveComparisonHTML(original, threshold, atkinson string) error {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Glyph Matching Comparison</title>
    <style>
        body {
            font-family: monospace;
            background: #f0f0f0;
        }
        .container {
            display: flex;
            gap: 20px;
            margin: 20px;
        }
        .panel {
            background: white;
            padding: 20px;
            border: 1px solid #ccc;
            flex: 1;
        }
        .title {
            font-size: 18px;
            font-weight: bold;
            margin-bottom: 10px;
        }
        .output {
            white-space: pre;
            line-height: 1.0;
            font-size: 8px;
            letter-spacing: 0;
        }
        img {
            max-width: 100%%;
            height: auto;
        }
    </style>
</head>
<body>
    <h1>Glyph Matching Results</h1>
    
    <div class="container">
        <div class="panel">
            <div class="title">Original Image</div>
            <img src="../../examples/baboon_256.png" alt="Original">
        </div>
        <div class="panel">
            <div class="title">Threshold Dithering</div>
            <img src="baboon_threshold.png" alt="Threshold">
        </div>
        <div class="panel">
            <div class="title">Atkinson Dithering</div>
            <img src="baboon_atkinson.png" alt="Atkinson">
        </div>
    </div>
    
    <div class="container">
        <div class="panel">
            <div class="title">Threshold Output</div>
            <div class="output">%s</div>
        </div>
        <div class="panel">
            <div class="title">Atkinson Output</div>
            <div class="output">%s</div>
        </div>
    </div>
</body>
</html>`, 
		strings.ReplaceAll(threshold, "\n", "<br>"),
		strings.ReplaceAll(atkinson, "\n", "<br>"))

	return os.WriteFile("comparison.html", []byte(html), 0644)
}

