package pairing

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf8"
)

const testURL = "http://192.168.1.24:7654/s/0JQ_LI4hPzL3isfD5U8wKw#token=jcuFWqzjZ0GJ469zo6tvp5kpKlptRfxGif6SWPiqzPM"

func TestEncodeProducesSquareQRCode(t *testing.T) {
	bitmap, err := encode(testURL)
	if err != nil {
		t.Fatalf("encode(): %v", err)
	}
	if len(bitmap) < 21 || bitmapWidth(bitmap) != len(bitmap) {
		t.Fatalf("encoded bitmap dimensions = %dx%d", bitmapWidth(bitmap), len(bitmap))
	}
	if !bitmap[0][0] || !bitmap[6][6] {
		t.Fatal("encoded bitmap is missing expected finder-pattern modules")
	}
}

func TestFormatFallbacksDoNotEncode(t *testing.T) {
	tests := []struct {
		name    string
		options Options
	}{
		{name: "non interactive", options: Options{Columns: 120}},
		{name: "dumb terminal", options: Options{Interactive: true, Columns: 120, Term: "dumb"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			output, err := format(testURL, test.options, func(string) ([][]bool, error) {
				called = true
				return nil, errors.New("must not encode")
			})
			if err != nil || called || output != "rome: transport "+testURL+"\n" {
				t.Fatalf("format() = (%q, %v), called=%t", output, err, called)
			}
		})
	}
}

func TestFormatFallsBackWhenTerminalIsNarrow(t *testing.T) {
	output, err := format(testURL, Options{Interactive: true, Columns: 8, Term: "xterm-256color"}, func(string) ([][]bool, error) {
		return [][]bool{{true}}, nil
	})
	if err != nil || output != "rome: transport "+testURL+"\n" {
		t.Fatalf("format() = (%q, %v)", output, err)
	}
}

func TestFormatRendersCredentialOnceAndResetsANSI(t *testing.T) {
	bitmap := [][]bool{
		{true, true, false},
		{true, false, true},
		{false, true, false},
	}
	output, err := format(testURL, Options{Interactive: true, Columns: 80, Term: "xterm-256color"}, func(content string) ([][]bool, error) {
		if content != testURL {
			t.Fatalf("encoder content = %q", content)
		}
		return bitmap, nil
	})
	if err != nil {
		t.Fatalf("format(): %v", err)
	}
	if !strings.HasPrefix(output, pairingTitle+"\n") || strings.Count(output, testURL) != 1 {
		t.Fatalf("pairing output did not contain the intended title and single URL: %q", output)
	}
	if got, want := strings.Count(output, ansiReset), (len(bitmap)+2*quietZoneModules+1)/2; got != want {
		t.Fatalf("ANSI reset count = %d, want %d", got, want)
	}
}

func TestFormatPropagatesEncodingFailureWithoutCredentialOutput(t *testing.T) {
	output, err := format(testURL, Options{Interactive: true, Columns: 120, Term: "xterm"}, func(string) ([][]bool, error) {
		return nil, errors.New("injected failure")
	})
	if err == nil || output != "" || strings.Contains(err.Error(), testURL) {
		t.Fatalf("format() = (%q, %v)", output, err)
	}
}

func TestFormatRejectsInvalidBitmap(t *testing.T) {
	output, err := format(testURL, Options{Interactive: true, Columns: 120, Term: "xterm"}, func(string) ([][]bool, error) {
		return [][]bool{{true}, {false, true}}, nil
	})
	if err == nil || output != "" || !strings.Contains(err.Error(), "invalid QR bitmap") {
		t.Fatalf("format() = (%q, %v)", output, err)
	}
}

func TestQRCodeDependencyDebugCannotLeakPairingData(t *testing.T) {
	if os.Getenv("ROME_QR_DEBUG_TEST_HELPER") == "1" {
		if got := os.Getenv("QRCODE_DEBUG"); got != "1" {
			t.Fatalf("QRCODE_DEBUG = %q, want child-visible value", got)
		}
		if _, err := encode(testURL); err != nil {
			t.Fatal(err)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestQRCodeDependencyDebugCannotLeakPairingData$")
	command.Env = append(os.Environ(), "ROME_QR_DEBUG_TEST_HELPER=1", "QRCODE_DEBUG=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("debug helper: %v\n%s", err, output)
	}
	if strings.Contains(string(output), testURL) || strings.Contains(string(output), "[qrcode] DEBUG") || strings.Contains(string(output), "bitsets") {
		t.Fatalf("QR dependency leaked pairing diagnostics: %q", output)
	}
}

func TestRenderBitmapAddsQuietZoneAndCombinesRows(t *testing.T) {
	bitmap := [][]bool{{true, false}, {false, true}}
	output := renderBitmap(bitmap)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if got, want := len(lines), (len(bitmap)+2*quietZoneModules+1)/2; got != want {
		t.Fatalf("rendered lines = %d, want %d", got, want)
	}
	for index, line := range lines {
		plain := stripANSI(line)
		if got, want := utf8.RuneCountInString(plain), len(bitmap[0])+2*quietZoneModules; got != want {
			t.Fatalf("line %d width = %d, want %d: %q", index, got, want, plain)
		}
	}
	if !strings.Contains(output, "▀") || !strings.Contains(output, "▄") {
		t.Fatalf("renderer omitted half-block combinations: %q", output)
	}
}

func stripANSI(value string) string {
	for _, sequence := range []string{ansiBlackOnWhite, "\x1b[40m", "\x1b[47m", ansiReset} {
		value = strings.ReplaceAll(value, sequence, "")
	}
	return value
}
