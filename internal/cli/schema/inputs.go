package schema

import (
	"encoding/json"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type inputSchema struct {
	Schema               string                `json:"$schema"`
	Type                 string                `json:"type"`
	Properties           map[string]flagSchema `json:"properties"`
	Required             []string              `json:"required,omitempty"`
	AdditionalProperties bool                  `json:"additionalProperties"`
}

type flagSchema struct {
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

func InputSchema(cmd *cobra.Command) string {
	s := inputSchema{
		Schema:               "https://json-schema.org/draft/2020-12/schema",
		Type:                 "object",
		Properties:           map[string]flagSchema{},
		AdditionalProperties: false,
	}

	visitFlags(cmd, func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		s.Properties[flag.Name] = flagSchema{
			Type:        schemaType(flag.Value.Type()),
			Default:     flag.DefValue,
			Description: flag.Usage,
		}
		if isRequired(flag) {
			s.Required = append(s.Required, flag.Name)
		}
	})
	sort.Strings(s.Required)

	bytes, err := json.Marshal(s)
	if err != nil {
		return `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{},"additionalProperties":false}`
	}
	return string(bytes)
}

func visitFlags(cmd *cobra.Command, visit func(*pflag.Flag)) {
	seen := map[string]bool{}
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		seen[flag.Name] = true
		visit(flag)
	})
	cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if seen[flag.Name] {
			return
		}
		visit(flag)
	})
}

func schemaType(flagType string) string {
	switch flagType {
	case "bool":
		return "boolean"
	case "int", "int64", "uint", "uint64":
		return "integer"
	case "float32", "float64":
		return "number"
	default:
		return "string"
	}
}

func isRequired(flag *pflag.Flag) bool {
	for key := range flag.Annotations {
		if key == cobra.BashCompOneRequiredFlag {
			return true
		}
		if key == "cobra_annotation_required" {
			return true
		}
	}
	return false
}
