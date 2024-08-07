package img2ansi

import (
	"fmt"
	"strings"
)

type AnsiEntry struct {
	Key   uint32
	Value string
}
type AnsiData []AnsiEntry

var (
	fgAnsi = NewOrderedMap()
	bgAnsi = NewOrderedMap()
)

// CompressANSI compresses an ANSI image by combining adjacent blocks with
// the same foreground and background colors. The function takes an ANSI
// image as a string and returns the more efficient ANSI image as a string.
func CompressANSI(ansiImage string) string {
	var compressed strings.Builder
	var currentFg, currentBg, currentBlock string
	var count int

	lines := strings.Split(ansiImage, "\n")
	for _, line := range lines {
		segments := strings.Split(line, "\u001b[")
		for _, segment := range segments {
			if segment == "" {
				continue
			}
			parts := strings.SplitN(segment, "m", 2)
			if len(parts) != 2 {
				continue
			}
			colorCode, block := parts[0], parts[1]
			fg, bg := extractColors(colorCode)

			// Optimize full block representation
			if block == "█" {
				bg = ""
			} else if block == " " {
				fg = ""
			}

			// If any color or block changes, write the current block
			// and start a new one
			if fg != currentFg || bg != currentBg || block != currentBlock {
				if count > 0 {
					compressed.WriteString(
						formatANSICode(
							currentFg, currentBg, currentBlock, count))
				}
				currentFg, currentBg, currentBlock = fg, bg, block
				count = 1
			} else {
				count++
			}
		}
		// Write the last block of the line
		if count > 0 {
			compressed.WriteString(
				formatANSICode(currentFg, currentBg, currentBlock, count))
		}
		compressed.WriteString(fmt.Sprintf("%s[0m\n", ESC))
		count = 0
		currentFg, currentBg = "", "" // Reset colors at end of line
	}

	return compressed.String()
}

// formatANSICode formats an ANSI color code with the given foreground and
// background colors, block character, and count. The function returns the
// ANSI color code as a string, with the foreground and background colors
// formatted as ANSI color codes, the block character repeated count times.
func formatANSICode(fg, bg, block string, count int) string {
	var code strings.Builder
	code.WriteString(ESC)
	code.WriteByte('[')
	if fg != "" {
		code.WriteString(fg)
		if bg != "" {
			code.WriteByte(';')
		}
	}
	if bg != "" {
		code.WriteString(bg)
	}
	code.WriteByte('m')
	code.WriteString(strings.Repeat(block, count))
	return code.String()
}

// extractColors extracts the foreground and background color codes from
// an ANSI color code. The function takes an ANSI color code as a string
// and returns the foreground and background color codes as strings.
func extractColors(colorCodes string) (fg string, bg string) {
	colors := strings.Split(colorCodes, ";")
	for i := 0; i < len(colors); i++ {
		if colors[i] == "38" && i+2 < len(colors) && colors[i+1] == "5" {
			fg = fmt.Sprintf("38;5;%s", colors[i+2])
			i += 2
		} else if colors[i] == "48" && i+2 < len(colors) && colors[i+1] == "5" {
			bg = fmt.Sprintf("48;5;%s", colors[i+2])
			i += 2
		} else if colorIsForeground(colors[i]) {
			fg = colors[i]
		} else if colorIsBackground(colors[i]) {
			bg = colors[i]
		}
	}
	return fg, bg
}

// colorIsForeground returns true if the ANSI color code corresponds to a
// foreground color, and false otherwise. The function takes an ANSI color
// code as a string and returns true if the color is a foreground color, and
// false if it is a background color.
func colorIsForeground(color string) bool {
	return strings.HasPrefix(color, "3") ||
		strings.HasPrefix(color, "9") ||
		color == "38"
}

// colorIsBackground returns true if the ANSI color code corresponds to a
// background color, and false otherwise. The function takes an ANSI color
// code as a string and returns true if the color is a background color, and
// false if it is a foreground color.
func colorIsBackground(color string) bool {
	return strings.HasPrefix(color, "4") ||
		strings.HasPrefix(color, "10") ||
		color == "48"
}

// ToOrderedMap converts an AnsiData slice to an OrderedMap  with the values
// as keys and the keys as values.
func (ansiData AnsiData) ToOrderedMap() *OrderedMap {
	om := NewOrderedMap()
	for _, entry := range ansiData {
		om.Set(entry.Key, entry.Value)
	}
	return om
}

// renderToAnsi renders a 2D array of BlockRune structs to an ANSI string.
// It does not perform any compression or optimization.
func renderToAnsi(blocks [][]BlockRune) string {
	var sb strings.Builder

	for _, row := range blocks {
		for _, block := range row {
			fgCode, _ := fgAnsi.Get(block.FG.toUint32())
			bgCode, _ := bgAnsi.Get(block.BG.toUint32())

			sb.WriteString(fmt.Sprintf("\x1b[%s;%sm", fgCode, bgCode))
			sb.WriteRune(block.Rune)
		}
		// Reset colors at the end of each line and add a newline
		sb.WriteString("\x1b[0m\n")
	}

	return sb.String()
}
