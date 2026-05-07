package schema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func RenderMarkdown(rootCmd *cobra.Command, registry map[string]string) string {
	var rows []string
	walk(rootCmd, func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		output := "no"
		if _, ok := registry[cmd.CommandPath()]; ok {
			output = "yes"
		}
		var flags []string
		visitFlags(cmd, func(flag *pflag.Flag) {
			if flag.Hidden {
				return
			}
			flags = append(flags, "--"+flag.Name)
		})
		sort.Strings(flags)
		rows = append(rows, fmt.Sprintf("| `%s` | %s | %s |", cmd.CommandPath(), strings.Join(flags, ", "), output))
	})
	sort.Strings(rows)

	var b strings.Builder
	b.WriteString("| Command | Flags | Output schema |\n")
	b.WriteString("|---|---|---|\n")
	for _, row := range rows {
		b.WriteString(row)
		b.WriteByte('\n')
	}
	return b.String()
}

func walk(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		walk(child, visit)
	}
}
