package fields

import (
	"strings"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

type FieldSet[T any] map[string]func(T) any

func Project[T any](set FieldSet[T], selected []string, item T) (map[string]any, error) {
	if len(selected) == 0 {
		selected = make([]string, 0, len(set))
		for name := range set {
			selected = append(selected, name)
		}
	}

	projected := make(map[string]any, len(selected))
	for _, field := range selected {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		fn, ok := set[field]
		if !ok {
			return nil, &clierrors.Error{
				Code:        "USAGE_UNKNOWN_FIELD",
				Message:     "unknown field: " + field,
				Remediation: "scribe schema <command> --fields",
				Exit:        clierrors.ExitUsage,
			}
		}
		projected[field] = fn(item)
	}
	return projected, nil
}
