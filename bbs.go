package img2ansi

import (
	"strconv"
	"strings"
)

// unicodeToCP437 maps Unicode block characters to their CP437 byte equivalents.
var unicodeToCP437 = map[rune]byte{
	' ': 0x20, // Space
	'█': 0xDB, // Full block
	'▄': 0xDC, // Lower half block
	'▌': 0xDD, // Left half block
	'▐': 0xDE, // Right half block
	'▀': 0xDF, // Upper half block
}

// bbsSGR builds the SGR escape sequence for a foreground/background code
// pair in legacy BBS form. Every sequence starts with an explicit reset
// so bold and blink never stick between runs. Bright foregrounds (90-97)
// become bold plus the standard color ("1;3x"). Bright backgrounds
// (100-107) become blink plus the standard color ("5;4x") when iCE colors
// are enabled, and fall back to the standard color otherwise.
func bbsSGR(fg, bg string, ice bool) string {
	var sb strings.Builder
	sb.WriteString("\x1b[0")
	if n, err := strconv.Atoi(fg); err == nil && n >= 90 && n <= 97 {
		sb.WriteString(";1")
		fg = strconv.Itoa(n - 60)
	}
	if n, err := strconv.Atoi(bg); err == nil && n >= 100 && n <= 107 {
		if ice {
			sb.WriteString(";5")
		}
		bg = strconv.Itoa(n - 60)
	}
	sb.WriteByte(';')
	sb.WriteString(fg)
	sb.WriteByte(';')
	sb.WriteString(bg)
	sb.WriteByte('m')
	return sb.String()
}

// CompressBBS renders block data to BBS-compatible ANSI art with CP437
// encoding. Output uses legacy escape codes (bold for bright foregrounds,
// blink for bright backgrounds under iCE), CR+LF line endings, and CP437
// bytes for block characters. Adjacent blocks with identical attributes
// share a single escape sequence. Returns []byte since CP437 is not
// valid UTF-8.
func (r *Renderer) CompressBBS(blocks [][]BlockRune) []byte {
	var buf []byte

	for _, row := range blocks {
		// Each line starts from the reset state, so the first block
		// always emits its attributes.
		currentSGR := ""
		for _, block := range row {
			// Block colors come from the palette search, so lookups only
			// miss if the palette was swapped after rendering; fall back
			// to white-on-black rather than panicking.
			fg, bg := "37", "40"
			if code, ok := r.fgAnsi.Get(block.FG.toUint32()); ok {
				fg = code.(string)
			}
			if code, ok := r.bgAnsi.Get(block.BG.toUint32()); ok {
				bg = code.(string)
			}

			if sgr := bbsSGR(fg, bg, r.ICEColors); sgr != currentSGR {
				buf = append(buf, sgr...)
				currentSGR = sgr
			}

			ch, ok := unicodeToCP437[block.Rune]
			if !ok {
				ch = 0x20
			}
			buf = append(buf, ch)
		}
		// Reset and CR+LF so colors never bleed across line wraps
		buf = append(buf, "\x1b[0m\r\n"...)
	}

	return buf
}
