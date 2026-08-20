package pairing

import "strings"

const (
	ansiBlackOnWhite = "\x1b[30;47m"
	ansiBlackBlock   = "\x1b[40m \x1b[47m"
	ansiReset        = "\x1b[0m"
)

// renderBitmap adds the QR quiet zone and combines two vertical modules into
// each terminal cell so the result remains square in a typical monospace font.
func renderBitmap(bitmap [][]bool) string {
	width := bitmapWidth(bitmap)
	if width == 0 || len(bitmap) != width {
		return ""
	}

	totalWidth := width + 2*quietZoneModules
	totalHeight := len(bitmap) + 2*quietZoneModules
	var output strings.Builder
	for top := 0; top < totalHeight; top += 2 {
		output.WriteString(ansiBlackOnWhite)
		for x := 0; x < totalWidth; x++ {
			topDark := moduleAt(bitmap, x-quietZoneModules, top-quietZoneModules)
			bottomDark := moduleAt(bitmap, x-quietZoneModules, top+1-quietZoneModules)
			switch {
			case topDark && bottomDark:
				output.WriteString(ansiBlackBlock)
			case topDark:
				output.WriteRune('▀')
			case bottomDark:
				output.WriteRune('▄')
			default:
				output.WriteByte(' ')
			}
		}
		output.WriteString(ansiReset)
		output.WriteByte('\n')
	}
	return output.String()
}

func moduleAt(bitmap [][]bool, x, y int) bool {
	return y >= 0 && y < len(bitmap) && x >= 0 && x < len(bitmap[y]) && bitmap[y][x]
}
