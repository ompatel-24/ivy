package pairing

import (
	"os"
	"sync"

	qrcode "github.com/yeqown/go-qrcode/v2"
)

var disableDependencyDebugOnce sync.Once

// The QR dependency has a package-level QRCODE_DEBUG hook that prints encoded
// bitsets. Prime that hook as disabled only when Ivy first needs a QR code,
// then restore the environment. The child process has already inherited the
// original environment by this point.
func disableDependencyDebug() {
	disableDependencyDebugOnce.Do(func() {
		value, present := os.LookupEnv("QRCODE_DEBUG")
		_ = os.Unsetenv("QRCODE_DEBUG")
		_, _ = qrcode.NewWith("ivy", qrcode.WithErrorCorrectionLevel(qrcode.ErrorCorrectionMedium))
		if present {
			_ = os.Setenv("QRCODE_DEBUG", value)
		}
	})
}
