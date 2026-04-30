package env

import (
	"os"

	"github.com/mattn/go-isatty"
)

type Format string

const (
	FormatJSON  Format = "json"
	FormatText  Format = "text"
	FormatQuiet Format = "quiet"
)

type Mode struct {
	Format      Format
	Color       bool
	Interactive bool
}

func Detect(stdout, stdin *os.File, jsonFlag bool) Mode {
	ci := os.Getenv("CI") == "true"
	stdoutTTY := isTTY(stdout)
	stdinTTY := isTTY(stdin)

	format := FormatText
	if jsonFlag || ci || !stdoutTTY {
		format = FormatJSON
	}

	return Mode{
		Format:      format,
		Color:       stdoutTTY && !ci && os.Getenv("NO_COLOR") != "1",
		Interactive: stdinTTY && !ci,
	}
}

func isTTY(file *os.File) bool {
	if file == nil {
		return false
	}
	return isatty.IsTerminal(file.Fd())
}
