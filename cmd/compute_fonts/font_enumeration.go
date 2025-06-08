package main

import (
	"fmt"
	"sort"
	"unicode"
)

// EnumerationOptions controls what types of characters are included
type EnumerationOptions struct {
	IncludeAlphanumeric bool // Include letters and numbers
	IncludeSemantic     bool // Include symbols with semantic meaning
	GeometricOnly       bool // Only geometric/pattern characters
	MinDensity          float64
	MaxDensity          float64
}

// CharacterCategory represents a category of characters
type CharacterCategory struct {
	Name       string
	Characters []CharacterInfo
	AvgDensity float64
	MinDensity float64
	MaxDensity float64
}

// CharacterInfo holds character data with properties
type CharacterInfo struct {
	Rune       rune
	Properties CharacterProperties
}

// EnumerateFontCharacters analyzes and categorizes all characters in a font
func EnumerateFontCharacters(glyphs []GlyphInfo, options EnumerationOptions) map[string]*CharacterCategory {
	categories := make(map[string]*CharacterCategory)
	
	// Initialize categories based on options
	if !options.GeometricOnly {
		if options.IncludeAlphanumeric {
			categories["alphanumeric"] = &CharacterCategory{Name: "Alphanumeric"}
		}
		if options.IncludeSemantic {
			categories["symbols"] = &CharacterCategory{Name: "Symbols"}
			categories["punctuation"] = &CharacterCategory{Name: "Punctuation"}
			categories["currency"] = &CharacterCategory{Name: "Currency"}
		}
	}
	
	// Always include pattern categories
	categories["blocks_solid"] = &CharacterCategory{Name: "Solid Blocks"}
	categories["blocks_partial"] = &CharacterCategory{Name: "Partial Blocks"}
	categories["shading"] = &CharacterCategory{Name: "Shading Patterns"}
	categories["lines_straight"] = &CharacterCategory{Name: "Straight Lines"}
	categories["lines_curved"] = &CharacterCategory{Name: "Curved Lines"}
	categories["dots_scattered"] = &CharacterCategory{Name: "Scattered Dots"}
	categories["geometric"] = &CharacterCategory{Name: "Geometric Shapes"}
	categories["box_drawing"] = &CharacterCategory{Name: "Box Drawing"}
	categories["texture"] = &CharacterCategory{Name: "Textures"}
	categories["pattern_regular"] = &CharacterCategory{Name: "Regular Patterns"}
	categories["gradient"] = &CharacterCategory{Name: "Gradients"}
	categories["outline"] = &CharacterCategory{Name: "Outlines"}
	categories["other"] = &CharacterCategory{Name: "Other"}
	
	// Process each glyph
	for _, glyph := range glyphs {
		// Skip if outside density range
		density := float64(glyph.Weight) / 64.0
		if options.MinDensity > 0 && density < options.MinDensity {
			continue
		}
		if options.MaxDensity > 0 && density > options.MaxDensity {
			continue
		}
		
		// Apply filters based on options
		if options.GeometricOnly && !isGeometricCharacter(glyph.Rune) {
			continue
		}
		
		// Analyze character properties
		props := AnalyzeCharacter(glyph)
		charInfo := CharacterInfo{
			Rune:       glyph.Rune,
			Properties: props,
		}
		
		// Classify character
		category := classifyCharacter(charInfo, options)
		if cat, exists := categories[category]; exists {
			cat.Characters = append(cat.Characters, charInfo)
		}
	}
	
	// Calculate statistics for each category
	for _, cat := range categories {
		if len(cat.Characters) == 0 {
			continue
		}
		
		var totalDensity float64
		cat.MinDensity = 1.0
		cat.MaxDensity = 0.0
		
		for _, char := range cat.Characters {
			density := char.Properties.Density
			totalDensity += density
			if density < cat.MinDensity {
				cat.MinDensity = density
			}
			if density > cat.MaxDensity {
				cat.MaxDensity = density
			}
		}
		
		cat.AvgDensity = totalDensity / float64(len(cat.Characters))
		
		// Sort characters by density
		sort.Slice(cat.Characters, func(i, j int) bool {
			return cat.Characters[i].Properties.Density < cat.Characters[j].Properties.Density
		})
	}
	
	// Remove empty categories
	for key, cat := range categories {
		if len(cat.Characters) == 0 {
			delete(categories, key)
		}
	}
	
	return categories
}

// classifyCharacter assigns a character to a category
func classifyCharacter(char CharacterInfo, options EnumerationOptions) string {
	r := char.Rune
	props := char.Properties
	
	// Handle alphanumeric first if included
	if !options.GeometricOnly && options.IncludeAlphanumeric {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return "alphanumeric"
		}
	}
	
	// Handle semantic symbols if included
	if !options.GeometricOnly && options.IncludeSemantic {
		if unicode.IsPunct(r) {
			return "punctuation"
		}
		if unicode.IsSymbol(r) {
			switch {
			case r >= 0x20A0 && r <= 0x20CF:
				return "currency"
			default:
				return "symbols"
			}
		}
	}
	
	// Pattern-based classification
	switch props.PatternType {
	case "solid":
		if props.Density > 0.9 {
			return "blocks_solid"
		} else {
			return "blocks_partial"
		}
	case "outline":
		return "outline"
	case "dotted", "scattered":
		return "dots_scattered"
	case "striped":
		if props.Orientation == "horizontal" || props.Orientation == "vertical" {
			return "lines_straight"
		}
		return "pattern_regular"
	case "gradient":
		return "gradient"
	}
	
	// Unicode block-based classification
	switch {
	case r >= 0x2500 && r <= 0x257F:
		return "box_drawing"
	case r >= 0x2580 && r <= 0x259F:
		if props.Density < 0.4 {
			return "shading"
		} else {
			return "blocks_partial"
		}
	case r >= 0x25A0 && r <= 0x25FF:
		return "geometric"
	}
	
	// Shape-based classification
	if props.ConnectedComponents == 1 {
		if props.Orientation == "horizontal" || props.Orientation == "vertical" {
			return "lines_straight"
		} else if props.Orientation == "circular" {
			return "geometric"
		}
	}
	
	// Texture detection
	if props.EdgeToFillRatio > 0.5 && props.ConnectedComponents > 3 {
		return "texture"
	}
	
	// Default
	return "other"
}

// isGeometricCharacter checks if a character is geometric/pattern-based
func isGeometricCharacter(r rune) bool {
	// Exclude control characters
	if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
		return false
	}
	
	// Exclude basic ASCII letters and numbers
	if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
		return false
	}
	
	// Exclude common punctuation with semantic meaning
	excludePunctuation := "!\"'(),-./:;?[]{}@#$%^&*+=<>\\|~`"
	for _, p := range excludePunctuation {
		if r == p {
			return false
		}
	}
	
	// Exclude extended alphabets
	if unicode.IsLetter(r) {
		return false
	}
	
	// Exclude specific semantic symbols
	switch r {
	case '©', '®', '™', '°', '¢', '£', '¥', '€', '¤':
		return false
	case '♠', '♣', '♥', '♦': // Card suits
		return false
	case '☺', '☻', '♪', '♫': // Smileys and music
		return false
	case '←', '→', '↑', '↓': // Arrows (semantic)
		return false
	}
	
	// Include geometric ranges
	switch {
	case r >= 0x2500 && r <= 0x257F: // Box Drawing
		return true
	case r >= 0x2580 && r <= 0x259F: // Block Elements
		return true
	case r >= 0x25A0 && r <= 0x25FF: // Geometric Shapes
		return true
	case r >= 0x2600 && r <= 0x26FF: // Miscellaneous Symbols (some geometric)
		return true
	case r >= 0x2700 && r <= 0x27BF: // Dingbats (some geometric)
		return true
	case r >= 0x2800 && r <= 0x28FF: // Braille Patterns
		return true
	}
	
	// Default to including if it's a symbol but not explicitly excluded
	return unicode.IsSymbol(r) || unicode.IsPunct(r)
}

// PrintFontSummary prints a summary of font enumeration results
func PrintFontSummary(categories map[string]*CharacterCategory) {
	// Sort categories by name
	var catNames []string
	for name := range categories {
		catNames = append(catNames, name)
	}
	sort.Strings(catNames)
	
	fmt.Println("\n=== Font Character Analysis ===")
	fmt.Printf("Total categories: %d\n\n", len(categories))
	
	for _, name := range catNames {
		cat := categories[name]
		fmt.Printf("%s (%d chars)\n", cat.Name, len(cat.Characters))
		fmt.Printf("  Density range: %.1f%% - %.1f%% (avg: %.1f%%)\n",
			cat.MinDensity*100, cat.MaxDensity*100, cat.AvgDensity*100)
		
		// Show sample characters
		if len(cat.Characters) > 0 {
			fmt.Printf("  Samples: ")
			count := 0
			for _, char := range cat.Characters {
				if count >= 10 {
					fmt.Printf("...")
					break
				}
				if char.Rune >= 0x20 && char.Rune != 0x7F {
					fmt.Printf("%c ", char.Rune)
					count++
				}
			}
			fmt.Println()
		}
		fmt.Println()
	}
}

// GetGeometricCharacters returns only geometric/pattern characters
func GetGeometricCharacters(glyphs []GlyphInfo) []rune {
	options := EnumerationOptions{
		GeometricOnly: true,
		MinDensity:    0.01, // Exclude empty characters
		MaxDensity:    0.99, // Exclude fully filled characters
	}
	
	categories := EnumerateFontCharacters(glyphs, options)
	
	var geometricChars []rune
	for _, cat := range categories {
		for _, char := range cat.Characters {
			geometricChars = append(geometricChars, char.Rune)
		}
	}
	
	// Sort by Unicode value for consistency
	sort.Slice(geometricChars, func(i, j int) bool {
		return geometricChars[i] < geometricChars[j]
	})
	
	return geometricChars
}

// GetPatternCharactersByDensity returns pattern characters grouped by density
func GetPatternCharactersByDensity(glyphs []GlyphInfo) map[string][]rune {
	options := EnumerationOptions{
		GeometricOnly: true,
	}
	
	categories := EnumerateFontCharacters(glyphs, options)
	
	// Group by density ranges
	densityGroups := map[string][]rune{
		"minimal":  {}, // 0-20%
		"light":    {}, // 20-40%
		"medium":   {}, // 40-60%
		"heavy":    {}, // 60-80%
		"dense":    {}, // 80-100%
	}
	
	for _, cat := range categories {
		for _, char := range cat.Characters {
			density := char.Properties.Density
			switch {
			case density < 0.2:
				densityGroups["minimal"] = append(densityGroups["minimal"], char.Rune)
			case density < 0.4:
				densityGroups["light"] = append(densityGroups["light"], char.Rune)
			case density < 0.6:
				densityGroups["medium"] = append(densityGroups["medium"], char.Rune)
			case density < 0.8:
				densityGroups["heavy"] = append(densityGroups["heavy"], char.Rune)
			default:
				densityGroups["dense"] = append(densityGroups["dense"], char.Rune)
			}
		}
	}
	
	return densityGroups
}