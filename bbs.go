package img2ansi

import (
	"fmt"
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

// bbsFgCode converts a modern ANSI foreground color code to BBS format.
// Standard colors (30-37) get "0;" prefix to clear sticky bold.
// Bright colors (90-97) become "1;3x" (bold + standard color).
func bbsFgCode(code string) string {
	switch code {
	case "30":
		return "0;30"
	case "31":
		return "0;31"
	case "32":
		return "0;32"
	case "33":
		return "0;33"
	case "34":
		return "0;34"
	case "35":
		return "0;35"
	case "36":
		return "0;36"
	case "37":
		return "0;37"
	case "90":
		return "1;30"
	case "91":
		return "1;31"
	case "92":
		return "1;32"
	case "93":
		return "1;33"
	case "94":
		return "1;34"
	case "95":
		return "1;35"
	case "96":
		return "1;36"
	case "97":
		return "1;37"
	default:
		return code
	}
}

// bbsBgCode converts a modern ANSI background color code to BBS format.
// Standard colors (40-47) pass through as-is.
// Bright colors (100-107) become "5;4x" (blink + standard bg) when iCE colors
// are enabled, otherwise fall back to the standard color (40-47).
func bbsBgCode(code string, ice bool) string {
	switch code {
	case "40", "41", "42", "43", "44", "45", "46", "47":
		return code
	case "100":
		if ice {
			return "5;40"
		}
		return "40"
	case "101":
		if ice {
			return "5;41"
		}
		return "41"
	case "102":
		if ice {
			return "5;42"
		}
		return "42"
	case "103":
		if ice {
			return "5;43"
		}
		return "43"
	case "104":
		if ice {
			return "5;44"
		}
		return "44"
	case "105":
		if ice {
			return "5;45"
		}
		return "45"
	case "106":
		if ice {
			return "5;46"
		}
		return "46"
	case "107":
		if ice {
			return "5;47"
		}
		return "47"
	default:
		return code
	}
}

// CompressBBS renders block data to BBS-compatible ANSI art with CP437 encoding.
// Output uses legacy escape codes (ESC[1;3xm for bright FG, ESC[5;4xm for bright BG),
// CR+LF line endings, and CP437 bytes for block characters.
// Returns []byte since CP437 is not valid UTF-8.
func (r *Renderer) CompressBBS(blocks [][]BlockRune) []byte {
	var buf []byte

	var currentFg, currentBg string
	var pendingChars []byte

	flush := func() {
		if len(pendingChars) == 0 {
			return
		}
		buf = append(buf, pendingChars...)
		pendingChars = pendingChars[:0]
	}

	writeColor := func(fg, bg string) {
		// Build full attribute set to avoid sticky bold/blink issues
		buf = append(buf, fmt.Sprintf("\x1b[0;%s;%sm", fg, bg)...)
		currentFg = fg
		currentBg = bg
	}

	for _, row := range blocks {
		currentFg = ""
		currentBg = ""
		pendingChars = pendingChars[:0]

		for _, block := range row {
			fgCode, _ := r.fgAnsi.Get(block.FG.toUint32())
			bgCode, _ := r.bgAnsi.Get(block.BG.toUint32())

			fg := bbsFgCode(fgCode.(string))
			bg := bbsBgCode(bgCode.(string), r.ICEColors)

			cp437Byte, ok := unicodeToCP437[block.Rune]
			if !ok {
				// Fallback: use space for any unmapped character
				cp437Byte = 0x20
			}

			// Optimize: full block only needs bg color (render as space with bg)
			// and space only needs... well, space with bg color
			if block.Rune == '█' {
				// Full block: render as space with fg as bg
				fgAsBg := bbsBgCode(bgCodeFromFg(fgCode.(string)), r.ICEColors)
				if currentFg != "0;30" || currentBg != fgAsBg {
					flush()
					writeColor("0;30", fgAsBg)
				}
				pendingChars = append(pendingChars, 0x20)
				continue
			}

			if fg != currentFg || bg != currentBg {
				flush()
				writeColor(fg, bg)
			}
			pendingChars = append(pendingChars, cp437Byte)
		}

		flush()
		// Reset and CR+LF
		buf = append(buf, "\x1b[0m\r\n"...)
	}

	return buf
}

// bgCodeFromFg converts a foreground ANSI code to the equivalent background code.
// e.g., "31" -> "41", "91" -> "101"
func bgCodeFromFg(fgCode string) string {
	switch fgCode {
	case "30":
		return "40"
	case "31":
		return "41"
	case "32":
		return "42"
	case "33":
		return "43"
	case "34":
		return "44"
	case "35":
		return "45"
	case "36":
		return "46"
	case "37":
		return "47"
	case "90":
		return "100"
	case "91":
		return "101"
	case "92":
		return "102"
	case "93":
		return "103"
	case "94":
		return "104"
	case "95":
		return "105"
	case "96":
		return "106"
	case "97":
		return "107"
	default:
		return "40"
	}
}
