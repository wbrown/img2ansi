package main

import (
	"math"
	"os"
	"strings"
	"testing"
)

// CharacterStats holds character frequency information
type CharacterStats struct {
	Char  rune
	Count int
}

// TestCompareRenderingMethods compares character distribution across rendering methods
func TestCompareRenderingMethods(t *testing.T) {
	testFiles := []struct {
		name        string
		file        string
		description string
	}{
		{"Naive 2-color", OutputPath("mandrill_brown_8x8.ans"), "Simple color extraction"},
		{"Exhaustive 16", OutputPath("mandrill_exhaustive_16.ans"), "Exhaustive search 16 colors"},
		{"Exhaustive 256", OutputPath("mandrill_exhaustive_256.ans"), "Exhaustive search 256 colors"},
		{"Quantized 4", OutputPath("mandrill_quantized_4.ans"), "4-color quantization"},
		{"Quantized 8", OutputPath("mandrill_quantized_8.ans"), "8-color quantization"},
		{"Optimized", OutputPath("mandrill_optimized.ans"), "Optimized pattern selection"},
	}
	
	results := make(map[string]map[rune]int)
	fileSizes := make(map[string]int64)
	
	// Analyze each file
	for _, tf := range testFiles {
		data, err := os.ReadFile(tf.file)
		if err != nil {
			t.Logf("Skipping %s: %v", tf.name, err)
			continue
		}
		
		fileSizes[tf.name] = int64(len(data))
		results[tf.name] = analyzeCharacterDistribution(string(data))
	}
	
	if len(results) == 0 {
		t.Skip("No test files found")
	}
	
	// Report results
	t.Log("\n=== Character Distribution Comparison ===")
	for _, tf := range testFiles {
		stats, exists := results[tf.name]
		if !exists {
			continue
		}
		
		t.Logf("\n%s (%s):", tf.name, tf.description)
		t.Logf("  File size: %d bytes", fileSizes[tf.name])
		
		// Get top characters
		topChars := getTopCharacters(stats, 5)
		for _, cs := range topChars {
			percent := float64(cs.Count) * 100.0 / float64(getTotalChars(stats))
			t.Logf("  '%c': %d (%.1f%%)", cs.Char, cs.Count, percent)
		}
		
		// Special focus on shading characters
		shadingChars := []rune{'░', '▒', '▓'}
		shadingTotal := 0
		for _, char := range shadingChars {
			if count, ok := stats[char]; ok {
				shadingTotal += count
			}
		}
		if shadingTotal > 0 {
			percent := float64(shadingTotal) * 100.0 / float64(getTotalChars(stats))
			t.Logf("  Shading chars total: %d (%.1f%%)", shadingTotal, percent)
		}
	}
}

// TestQuantizationEffects specifically tests quantization impact
func TestQuantizationEffects(t *testing.T) {
	quantizationTests := []struct {
		name   string
		file   string
		colors int
	}{
		{"No quantization", OutputPath("mandrill_exhaustive_16.ans"), 16},
		{"4 colors", OutputPath("mandrill_quantized_4.ans"), 4},
		{"8 colors", OutputPath("mandrill_quantized_8.ans"), 8},
	}
	
	shadingChars := []rune{' ', '░', '▒', '▓', '█'}
	
	for _, qt := range quantizationTests {
		data, err := os.ReadFile(qt.file)
		if err != nil {
			t.Logf("Skipping %s: %v", qt.name, err)
			continue
		}
		
		stats := analyzeCharacterDistribution(string(data))
		total := getTotalChars(stats)
		
		t.Logf("\n%s (%d colors):", qt.name, qt.colors)
		
		// Analyze shading character distribution
		for _, char := range shadingChars {
			if count, ok := stats[char]; ok && count > 0 {
				percent := float64(count) * 100.0 / float64(total)
				t.Logf("  '%c': %d (%.1f%%)", char, count, percent)
			}
		}
		
		// Calculate distribution entropy
		entropy := calculateEntropy(stats, shadingChars)
		t.Logf("  Shading entropy: %.2f bits", entropy)
	}
}

// BenchmarkCharacterAnalysis benchmarks character distribution analysis
func BenchmarkCharacterAnalysis(b *testing.B) {
	// Create a sample ANSI string
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("\x1b[38;5;15;48;5;0m")
		sb.WriteRune('▒')
	}
	testData := sb.String()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = analyzeCharacterDistribution(testData)
	}
}

// Helper functions

func analyzeCharacterDistribution(content string) map[rune]int {
	charCount := make(map[rune]int)
	
	// Remove ANSI escape sequences
	inEscape := false
	for _, r := range content {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		if r != '\n' && r != '\r' {
			charCount[r]++
		}
	}
	
	return charCount
}

func getTopCharacters(stats map[rune]int, n int) []CharacterStats {
	var chars []CharacterStats
	for char, count := range stats {
		chars = append(chars, CharacterStats{char, count})
	}
	
	// Sort by count
	for i := 0; i < len(chars)-1; i++ {
		for j := i + 1; j < len(chars); j++ {
			if chars[j].Count > chars[i].Count {
				chars[i], chars[j] = chars[j], chars[i]
			}
		}
	}
	
	if len(chars) > n {
		chars = chars[:n]
	}
	
	return chars
}

func getTotalChars(stats map[rune]int) int {
	total := 0
	for _, count := range stats {
		total += count
	}
	return total
}

func calculateEntropy(stats map[rune]int, chars []rune) float64 {
	total := 0
	for _, char := range chars {
		if count, ok := stats[char]; ok {
			total += count
		}
	}
	
	if total == 0 {
		return 0
	}
	
	entropy := 0.0
	for _, char := range chars {
		if count, ok := stats[char]; ok && count > 0 {
			p := float64(count) / float64(total)
			entropy -= p * math.Log2(p)
		}
	}
	
	return entropy
}

// Removed abs function - already defined in fonts.go