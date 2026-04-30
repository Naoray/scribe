package cmd

import clischema "github.com/Naoray/scribe/internal/cli/schema"

var connectOutputSchema = mutatorWorkflowOutputSchema

func init() {
	clischema.Register("scribe connect", connectOutputSchema)
	clischema.Register("scribe registry connect", connectOutputSchema)
}
