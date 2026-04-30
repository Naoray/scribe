package cmd

import "github.com/spf13/cobra"

func RootCommandForDocs() *cobra.Command {
	return newRootCmd()
}
