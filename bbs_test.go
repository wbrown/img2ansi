package img2ansi

import (
	"bytes"
	"testing"
)

func TestBbsSGR(t *testing.T) {
	tests := []struct {
		fg       string
		bg       string
		ice      bool
		expected string
	}{
		// Standard colors always lead with a reset so bold/blink never stick
		{"30", "40", false, "\x1b[0;30;40m"},
		{"37", "47", false, "\x1b[0;37;47m"},
		// Bright foregrounds become bold + standard color
		{"90", "40", false, "\x1b[0;1;30;40m"},
		{"91", "41", false, "\x1b[0;1;31;41m"},
		{"97", "47", false, "\x1b[0;1;37;47m"},
		// Bright backgrounds fall back to standard without iCE
		{"31", "100", false, "\x1b[0;31;40m"},
		{"31", "107", false, "\x1b[0;31;47m"},
		// Bright backgrounds become blink + standard color with iCE
		{"31", "100", true, "\x1b[0;5;31;40m"},
		{"31", "107", true, "\x1b[0;5;31;47m"},
		// Bold and blink combine
		{"91", "101", true, "\x1b[0;1;5;31;41m"},
	}

	for _, tt := range tests {
		result := bbsSGR(tt.fg, tt.bg, tt.ice)
		if result != tt.expected {
			t.Errorf("bbsSGR(%q, %q, %v) = %q, want %q",
				tt.fg, tt.bg, tt.ice, result, tt.expected)
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
	// iCE colors with the full ansi16 palette (16 background colors)
	r := NewRenderer(
		WithBBSMode(),
		WithICEColors(),
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

	// Verify escape codes use legacy BBS format: bright colors must be
	// expressed via bold/blink attributes, not modern 9x/10x codes
	if bytes.Contains(output, []byte("\x1b[91")) {
		t.Error("Output should not contain modern bright FG codes like ESC[91")
	}
	if bytes.Contains(output, []byte("101")) {
		t.Error("Output should not contain modern bright BG codes like 101")
	}

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
		WithBBSMode(),
		WithPalette("ansi16bbs"),
	)

	blocks := r.blocks

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

	blocks := r.blocks

	// Default should still have 16 blocks
	if len(blocks) != 16 {
		t.Errorf("Default mode should have 16 blocks, got %d", len(blocks))
	}
}

// Regression test: a full block with a bright foreground must keep its
// bold foreground and CP437 0xDB byte, not be rewritten to a space with
// a background color that silently loses brightness without iCE.
func TestCompressBBSBrightForegroundFullBlock(t *testing.T) {
	r := NewRenderer(
		WithBBSMode(),
		WithPalette("ansi16bbs"),
	)

	brightRed := RGB{0xFF, 0x55, 0x55} // code 91
	black := RGB{0x00, 0x00, 0x00}     // code 40

	blocks := [][]BlockRune{{{Rune: '█', FG: brightRed, BG: black}}}
	out := r.CompressBBS(blocks)

	if !bytes.Contains(out, []byte{0xDB}) {
		t.Errorf("full block should be emitted as CP437 0xDB, got %q", out)
	}
	if !bytes.Contains(out, []byte("1;31")) {
		t.Errorf("bright red foreground should be emitted as bold (1;31), got %q", out)
	}
}

// Regression test: without iCE colors only 8 background colors are
// expressible, so the ansi16bbs palette must keep the block search from
// choosing bright backgrounds that the output encoding would silently
// downgrade. A solid bright-red block must come back as a bright
// foreground on a full block, not a bright background under a space.
func TestBBSSearchUsesBoldForBrightSolid(t *testing.T) {
	r := NewRenderer(
		WithBBSMode(),
		WithPalette("ansi16bbs"),
	)

	brightRed := RGB{0xFF, 0x55, 0x55}
	block := [4]RGB{brightRed, brightRed, brightRed, brightRed}
	rn, fg, _ := r.FindBestBlockRepresentation(block, false)

	if rn != '█' || fg != brightRed {
		t.Errorf("solid bright red should map to '█' with bright red FG, got %q fg=%v",
			rn, fg)
	}
}

// The ansi16bbs palette must expose all 16 foreground colors but only
// the 8 standard background colors.
func TestANSI16BBSPaletteShape(t *testing.T) {
	r := NewRenderer(WithPalette("ansi16bbs"))

	fgCount := 0
	r.fgAnsi.Iterate(func(_, _ interface{}) { fgCount++ })
	bgCount := 0
	r.bgAnsi.Iterate(func(_, _ interface{}) { bgCount++ })

	if fgCount != 16 {
		t.Errorf("ansi16bbs should have 16 foreground colors, got %d", fgCount)
	}
	if bgCount != 8 {
		t.Errorf("ansi16bbs should have 8 background colors, got %d", bgCount)
	}
}
