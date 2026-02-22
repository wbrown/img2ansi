package img2ansi

import (
	"bytes"
	"testing"
)

func TestBbsFgCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Standard colors get 0; prefix to clear sticky bold
		{"30", "0;30"},
		{"31", "0;31"},
		{"32", "0;32"},
		{"33", "0;33"},
		{"34", "0;34"},
		{"35", "0;35"},
		{"36", "0;36"},
		{"37", "0;37"},
		// Bright colors become 1;3x (bold + standard)
		{"90", "1;30"},
		{"91", "1;31"},
		{"92", "1;32"},
		{"93", "1;33"},
		{"94", "1;34"},
		{"95", "1;35"},
		{"96", "1;36"},
		{"97", "1;37"},
	}

	for _, tt := range tests {
		result := bbsFgCode(tt.input)
		if result != tt.expected {
			t.Errorf("bbsFgCode(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestBbsBgCodeWithICE(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Standard BG colors pass through
		{"40", "40"},
		{"41", "41"},
		{"42", "42"},
		{"43", "43"},
		{"44", "44"},
		{"45", "45"},
		{"46", "46"},
		{"47", "47"},
		// Bright BG colors become 5;4x with iCE
		{"100", "5;40"},
		{"101", "5;41"},
		{"102", "5;42"},
		{"103", "5;43"},
		{"104", "5;44"},
		{"105", "5;45"},
		{"106", "5;46"},
		{"107", "5;47"},
	}

	for _, tt := range tests {
		result := bbsBgCode(tt.input, true)
		if result != tt.expected {
			t.Errorf("bbsBgCode(%q, true) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestBbsBgCodeWithoutICE(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Standard BG colors pass through
		{"40", "40"},
		{"47", "47"},
		// Bright BG colors fall back to standard without iCE
		{"100", "40"},
		{"101", "41"},
		{"102", "42"},
		{"103", "43"},
		{"104", "44"},
		{"105", "45"},
		{"106", "46"},
		{"107", "47"},
	}

	for _, tt := range tests {
		result := bbsBgCode(tt.input, false)
		if result != tt.expected {
			t.Errorf("bbsBgCode(%q, false) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestUnicodeToCP437Mapping(t *testing.T) {
	tests := []struct {
		r        rune
		expected byte
	}{
		{' ', 0x20},
		{'█', 0xDB},
		{'▄', 0xDC},
		{'▌', 0xDD},
		{'▐', 0xDE},
		{'▀', 0xDF},
	}

	for _, tt := range tests {
		b, ok := unicodeToCP437[tt.r]
		if !ok {
			t.Errorf("unicodeToCP437[%q] not found", tt.r)
			continue
		}
		if b != tt.expected {
			t.Errorf("unicodeToCP437[%q] = 0x%02X, want 0x%02X", tt.r, b, tt.expected)
		}
	}
}

func TestCompressBBSProducesValidOutput(t *testing.T) {
	// Create a simple renderer with ansi16 palette for testing
	r := NewRenderer(
		WithBBSMode(true),
		WithPalette("ansi16"),
	)

	// Use actual ANSI 16 palette colors (from ansi16.json)
	black := RGB{0x00, 0x00, 0x00}   // code 30/40
	red := RGB{0xAA, 0x00, 0x00}     // code 31/41
	green := RGB{0x00, 0xAA, 0x00}   // code 32/42
	blue := RGB{0x00, 0x00, 0xAA}    // code 34/44
	yellow := RGB{0xAA, 0x55, 0x00}  // code 33/43
	brightRed := RGB{0xFF, 0x55, 0x55} // code 91/101

	// Create a small test block grid (2x2 blocks)
	blocks := [][]BlockRune{
		{
			{Rune: '▀', FG: red, BG: black},
			{Rune: '█', FG: green, BG: black},
		},
		{
			{Rune: ' ', FG: black, BG: blue},
			{Rune: '▄', FG: yellow, BG: brightRed},
		},
	}

	output := r.CompressBBS(blocks)

	// Verify output is non-empty
	if len(output) == 0 {
		t.Fatal("CompressBBS produced empty output")
	}

	// Verify CR+LF line endings
	if !bytes.Contains(output, []byte("\r\n")) {
		t.Error("Output should contain CR+LF line endings")
	}

	// Verify block characters are single CP437 bytes, not multi-byte UTF-8.
	// Unicode block chars like '▀' are 3 bytes in UTF-8 (e.g., 0xE2 0x96 0x80).
	// In CP437 they're single bytes (0xDB-0xDF). Check that no 3-byte UTF-8
	// sequences for block characters appear in the output.
	unicodeBlockPrefix := []byte{0xE2, 0x96} // Common prefix for Unicode block elements
	if bytes.Contains(output, unicodeBlockPrefix) {
		t.Error("Output contains multi-byte UTF-8 block characters - should be CP437 single bytes")
	}

	// Verify escape codes use legacy BBS format
	// Should contain "0;" or "1;" prefixes, not bare "9x" codes
	outputStr := string(output)
	if bytes.Contains(output, []byte("\x1b[91")) {
		t.Error("Output should not contain modern bright FG codes like ESC[91")
	}
	_ = outputStr

	// Verify reset at end of each line
	lines := bytes.Split(output, []byte("\r\n"))
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		if !bytes.HasSuffix(line, []byte("\x1b[0m")) {
			t.Errorf("Line %d should end with ESC[0m reset", i)
		}
	}
}

func TestBBSModeBlockSet(t *testing.T) {
	r := NewRenderer(
		WithBBSMode(true),
		WithPalette("ansi16"),
	)

	blocks := r.getBlocks()

	// Should have exactly 6 blocks
	if len(blocks) != 6 {
		t.Errorf("BBS mode should have 6 blocks, got %d", len(blocks))
	}

	// All blocks should be in CP437
	for _, b := range blocks {
		if _, ok := unicodeToCP437[b.Rune]; !ok {
			t.Errorf("BBS block %q (U+%04X) not in CP437 map", b.Rune, b.Rune)
		}
	}
}

func TestDefaultBlockSetUnchanged(t *testing.T) {
	r := NewRenderer(
		WithPalette("ansi16"),
	)

	blocks := r.getBlocks()

	// Default should still have 16 blocks
	if len(blocks) != 16 {
		t.Errorf("Default mode should have 16 blocks, got %d", len(blocks))
	}
}

func TestBgCodeFromFg(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"30", "40"},
		{"31", "41"},
		{"37", "47"},
		{"90", "100"},
		{"91", "101"},
		{"97", "107"},
	}

	for _, tt := range tests {
		result := bgCodeFromFg(tt.input)
		if result != tt.expected {
			t.Errorf("bgCodeFromFg(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
