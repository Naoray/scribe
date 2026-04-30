package env

import (
	"os"
	"testing"
)

func TestDetect(t *testing.T) {
	t.Run("ci forces json", func(t *testing.T) {
		t.Setenv("CI", "true")
		mode := Detect(os.Stdout, os.Stdin, false)
		if mode.Format != FormatJSON {
			t.Fatalf("Format = %s, want json", mode.Format)
		}
	})

	t.Run("no color keeps text format", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		stdout, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer stdout.Close()
		mode := Detect(stdout, os.Stdin, false)
		if mode.Color {
			t.Fatal("Color = true, want false")
		}
		if mode.Format != FormatJSON {
			t.Fatalf("non-tty Format = %s, want json", mode.Format)
		}
	})

	t.Run("json overrides tty", func(t *testing.T) {
		mode := Detect(os.Stdout, os.Stdin, true)
		if mode.Format != FormatJSON {
			t.Fatalf("Format = %s, want json", mode.Format)
		}
	})
}
