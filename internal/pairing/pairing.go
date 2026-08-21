// Package pairing creates the local terminal handoff for Rome's authenticated
// browser Session. Pairing URLs are credentials and must never be logged.
package pairing

import (
	"fmt"
	"strings"

	qrcode "github.com/yeqown/go-qrcode/v2"
)

const (
	quietZoneModules = 4
	pairingTitle     = "🌿 Rome — scan to connect"
)

// Options describes whether the local output can display a QR code.
type Options struct {
	Interactive bool
	Columns     uint16
	Term        string
}

// Format returns either a compact QR pairing block or the machine-friendly
// one-line URL fallback used by non-interactive and narrow terminals.
func Format(connectionURL string, options Options) (string, error) {
	return format(connectionURL, options, encode)
}

type encoder func(string) ([][]bool, error)

func format(connectionURL string, options Options, encodeQR encoder) (string, error) {
	fallback := fmt.Sprintf("rome: transport %s\n", connectionURL)
	if !options.Interactive || strings.EqualFold(strings.TrimSpace(options.Term), "dumb") {
		return fallback, nil
	}

	bitmap, err := encodeQR(connectionURL)
	if err != nil {
		return "", fmt.Errorf("encode pairing URL: %w", err)
	}
	bitmapWidth := bitmapWidth(bitmap)
	if bitmapWidth == 0 || len(bitmap) != bitmapWidth {
		return "", fmt.Errorf("encode pairing URL: invalid QR bitmap")
	}
	width := bitmapWidth + 2*quietZoneModules
	if int(options.Columns) < width {
		return fallback, nil
	}

	return pairingTitle + "\n" + renderBitmap(bitmap) + fallback, nil
}

type matrixWriter struct {
	bitmap [][]bool
}

func (w *matrixWriter) Write(matrix qrcode.Matrix) error {
	w.bitmap = matrix.Bitmap()
	return nil
}

func (w *matrixWriter) Close() error {
	return nil
}

func encode(content string) ([][]bool, error) {
	disableDependencyDebug()
	code, err := qrcode.NewWith(content, qrcode.WithErrorCorrectionLevel(qrcode.ErrorCorrectionMedium))
	if err != nil {
		return nil, err
	}
	writer := &matrixWriter{}
	if err := code.Save(writer); err != nil {
		return nil, err
	}
	return writer.bitmap, nil
}

func bitmapWidth(bitmap [][]bool) int {
	if len(bitmap) == 0 {
		return 0
	}
	width := len(bitmap[0])
	for _, row := range bitmap {
		if len(row) != width {
			return 0
		}
	}
	return width
}
