package main

import (
	"fmt"
	"testing"
)

func TestRenderStrategies(t *testing.T) {
	// Test HalfBlockStrategy
	t.Run("HalfBlockStrategy", func(t *testing.T) {
		strategy := NewHalfBlockStrategy()
		
		tests := []struct {
			name   string
			pixels [][]uint8
			want   rune
		}{
			{"empty", [][]uint8{{0}, {0}}, ' '},
			{"full", [][]uint8{{255}, {255}}, '█'},
			{"upper", [][]uint8{{255}, {0}}, '▀'},
			{"lower", [][]uint8{{0}, {255}}, '▄'},
		}
		
		for _, tc := range tests {
			got, err := strategy.RenderBlock(tc.pixels)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			}
			if got != tc.want {
				t.Errorf("%s: got %c, want %c", tc.name, got, tc.want)
			}
		}
	})
	
	// Test ShadingStrategy
	t.Run("ShadingStrategy", func(t *testing.T) {
		strategy := NewShadingStrategy()
		
		tests := []struct {
			name   string
			pixels [][]uint8
			want   rune
		}{
			{"empty", [][]uint8{{0, 0}, {0, 0}}, ' '},
			{"light", [][]uint8{{80, 80}, {80, 80}}, '░'},
			{"medium", [][]uint8{{140, 140}, {140, 140}}, '▒'},
			{"dark", [][]uint8{{200, 200}, {200, 200}}, '▓'},
			{"full", [][]uint8{{255, 255}, {255, 255}}, '█'},
		}
		
		for _, tc := range tests {
			got, err := strategy.RenderBlock(tc.pixels)
			if err != nil {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			}
			if got != tc.want {
				t.Errorf("%s: got %c (U+%04X), want %c (U+%04X)", tc.name, got, got, tc.want, tc.want)
			}
		}
	})
}

func TestStrategySelection(t *testing.T) {
	// Load fonts
	IBMFont, err := loadFont("PxPlus_IBM_BIOS.ttf")
	if err != nil {
		t.Fatalf("Failed to load IBM BIOS font: %v", err)
	}
	
	safeFont, err := loadFont("FSD - PragmataProMono.ttf")
	if err != nil {
		t.Fatalf("Failed to load safe font: %v", err)
	}
	
	// Analyze font capabilities
	caps := AnalyzeFontCapabilities(IBMFont, "IBM BIOS")
	
	// Create glyph lookup
	glyphs := analyzeFont(IBMFont, safeFont)
	lookup := NewGlyphLookup(glyphs)
	
	// Select strategy
	strategy := SelectRenderStrategy(caps, lookup)
	
	// For IBM BIOS, we expect GlyphMatchingStrategy
	if _, ok := strategy.(*GlyphMatchingStrategy); !ok {
		t.Errorf("Expected GlyphMatchingStrategy for IBM BIOS font")
	}
	
	fmt.Printf("Selected strategy: %T\n", strategy)
	w, h := strategy.GetBlockSize()
	fmt.Printf("Block size: %dx%d\n", w, h)
}