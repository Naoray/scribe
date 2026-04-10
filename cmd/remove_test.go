package cmd

import (
	"strings"
	"testing"
)

func TestResolveRemoveTarget(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		installed []string
		want      string
		wantErr   string
	}{
		{
			name:      "exact namespaced match",
			input:     "Artistfy-hq/recap",
			installed: []string{"Artistfy-hq/recap", "antfu-skills/recap"},
			want:      "Artistfy-hq/recap",
		},
		{
			name:      "bare name unique match",
			input:     "deploy",
			installed: []string{"Artistfy-hq/deploy", "Artistfy-hq/recap"},
			want:      "Artistfy-hq/deploy",
		},
		{
			name:      "bare name ambiguous",
			input:     "recap",
			installed: []string{"Artistfy-hq/recap", "antfu-skills/recap"},
			wantErr:   "ambiguous",
		},
		{
			name:      "not found",
			input:     "nonexistent",
			installed: []string{"Artistfy-hq/recap"},
			wantErr:   "not installed",
		},
		{
			name:      "exact match case-sensitive",
			input:     "Artistfy-hq/deploy",
			installed: []string{"Artistfy-hq/deploy"},
			want:      "Artistfy-hq/deploy",
		},
		{
			name:      "bare name single installed",
			input:     "recap",
			installed: []string{"Artistfy-hq/recap"},
			want:      "Artistfy-hq/recap",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveRemoveTarget(tc.input, tc.installed)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
