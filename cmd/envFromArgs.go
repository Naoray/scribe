package cmd

import (
	"os"
	"strings"

	clienv "github.com/Naoray/scribe/internal/cli/env"
)

func envFromArgs(args []string) clienv.Mode {
	jsonFlag := false
	for _, arg := range args {
		if arg == "--json" || strings.HasPrefix(arg, "--json=") {
			jsonFlag = true
			break
		}
	}
	return clienv.Detect(os.Stdout, os.Stdin, jsonFlag)
}
