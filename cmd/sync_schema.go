package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var syncOutputSchema = mutatorWorkflowOutputSchema

func init() {
	clischema.Register("scribe sync", syncOutputSchema)
}
