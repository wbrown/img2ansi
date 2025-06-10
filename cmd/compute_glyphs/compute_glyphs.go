package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"flag"
	"fmt"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"github.com/wbrown/img2ansi"
	"golang.org/x/image/font"
	"image"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// FontGlyphData represents pre-computed glyph bitmaps for a font
type FontGlyphData struct {
	FontName string
	Glyphs   map[rune]img2ansi.GlyphBitmap
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
func renderGlyphToBitmap(ttfFont *truetype.Font, r rune) img2ansi.GlyphBitmap {
	// Create font face with proper size
	face := truetype.NewFace(ttfFont, &truetype.Options{
		Size:    float64(img2ansi.GlyphHeight), // 8 point size
		DPI:     72,
		Hinting: font.HintingFull,
	})
	defer face.Close()
	
	// Create an 8x8 alpha image (better for anti-aliasing)
	img := image.NewAlpha(image.Rect(0, 0, img2ansi.GlyphWidth, img2ansi.GlyphHeight))
	
	// Set up the freetype context
	ctx := freetype.NewContext()
	ctx.SetDPI(72)
	ctx.SetFont(ttfFont)
	ctx.SetFontSize(float64(img2ansi.GlyphHeight))
	ctx.SetClip(img.Bounds())
	ctx.SetDst(img)
	ctx.SetSrc(image.White)
	ctx.SetHinting(font.HintingFull)
	
	// Get glyph metrics for centering
	metrics := face.Metrics()
	
	// Calculate baseline position (approximate centering)
	ascent := metrics.Ascent >> 6   // Convert from 26.6 fixed point to pixels
	descent := metrics.Descent >> 6 // Descent is typically negative
	baselineY := (img2ansi.GlyphHeight + int(ascent) - int(descent)) / 2
	
	// Draw the character
	pt := freetype.Pt(0, baselineY)
	ctx.DrawString(string(r), pt)

	// Convert to bitmap using 25% alpha threshold
	var bitmap img2ansi.GlyphBitmap
	for y := 0; y < img2ansi.GlyphHeight; y++ {
		for x := 0; x < img2ansi.GlyphWidth; x++ {
			if img.AlphaAt(x, y).A > 64 { // 25% threshold
				bitmap |= 1 << (y*img2ansi.GlyphWidth + x)
			}
		}
	}

	return bitmap
}

// computeFontGlyphs pre-renders all glyphs for a font
func computeFontGlyphs(fontPath string) (*FontGlyphData, error) {
	font, err := loadFont(fontPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %w", err)
	}

	data := &FontGlyphData{
		FontName: filepath.Base(fontPath),
		Glyphs:   make(map[rune]img2ansi.GlyphBitmap),
	}

	// Pre-render ASCII printable characters
	for r := rune(32); r <= rune(126); r++ {
		data.Glyphs[r] = renderGlyphToBitmap(font, r)
	}

	// Add Unicode block characters
	blockChars := []rune{
		' ', '▀', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█',
		'▌', '▍', '▎', '▏', '▐', '░', '▒', '▓',
		'▔', '▕', '▖', '▗', '▘', '▙', '▚', '▛', '▜', '▝', '▞', '▟',
	}
	for _, r := range blockChars {
		data.Glyphs[r] = renderGlyphToBitmap(font, r)
	}

	// Add box drawing characters
	boxChars := []rune{
		'─', '━', '│', '┃', '┄', '┅', '┆', '┇', '┈', '┉', '┊', '┋',
		'┌', '┍', '┎', '┏', '┐', '┑', '┒', '┓',
		'└', '┕', '┖', '┗', '┘', '┙', '┚', '┛',
		'├', '┝', '┞', '┟', '┠', '┡', '┢', '┣',
		'┤', '┥', '┦', '┧', '┨', '┩', '┪', '┫',
		'┬', '┭', '┮', '┯', '┰', '┱', '┲', '┳',
		'┴', '┵', '┶', '┷', '┸', '┹', '┺', '┻',
		'┼', '┽', '┾', '┿', '╀', '╁', '╂', '╃', '╄', '╅', '╆', '╇', '╈', '╉', '╊', '╋',
		'╌', '╍', '╎', '╏',
		'═', '║', '╒', '╓', '╔', '╕', '╖', '╗',
		'╘', '╙', '╚', '╛', '╜', '╝', '╞', '╟',
		'╠', '╡', '╢', '╣', '╤', '╥', '╦', '╧',
		'╨', '╩', '╪', '╫', '╬',
	}
	for _, r := range boxChars {
		data.Glyphs[r] = renderGlyphToBitmap(font, r)
	}

	return data, nil
}

// saveFontGlyphData saves pre-computed glyph data to a binary file
func saveFontGlyphData(data *FontGlyphData, outputPath string) error {
	// Create a buffer for the compressed data
	var buf bytes.Buffer
	
	// Create gzip writer
	gz := gzip.NewWriter(&buf)
	
	// Create gob encoder
	enc := gob.NewEncoder(gz)
	
	// Encode the data
	if err := enc.Encode(data); err != nil {
		gz.Close()
		return fmt.Errorf("failed to encode data: %w", err)
	}
	
	// Close gzip writer
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to close gzip: %w", err)
	}
	
	// Write to file
	if err := ioutil.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	return nil
}

func main() {
	inputFont := flag.String("font", "", "Path to the input font file (required)")
	outputFile := flag.String("output", "", "Path to save the output glyph data file (required)")
	flag.Parse()

	if *inputFont == "" || *outputFile == "" {
		fmt.Println("Both -font and -output flags are required")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.Printf("Computing glyphs for font: %s", *inputFont)
	
	// Compute font glyphs
	data, err := computeFontGlyphs(*inputFont)
	if err != nil {
		log.Fatalf("Failed to compute glyphs: %v", err)
	}
	
	log.Printf("Computed %d glyphs", len(data.Glyphs))
	
	// Save the data
	if err := saveFontGlyphData(data, *outputFile); err != nil {
		log.Fatalf("Failed to save glyph data: %v", err)
	}
	
	// Report file size
	fileInfo, err := os.Stat(*outputFile)
	if err == nil {
		log.Printf("Saved glyph data to %s (%.2f KB)", *outputFile, float64(fileInfo.Size())/1024)
	}
	
	// Generate a simple name for embedding
	baseName := strings.TrimSuffix(filepath.Base(*inputFont), filepath.Ext(*inputFont))
	suggestedName := strings.ToLower(strings.ReplaceAll(baseName, " ", "_")) + ".glyphs"
	log.Printf("Suggested embedded filename: fontdata/%s", suggestedName)
}