package envelope

import (
	"encoding/json"
	"testing"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

func TestEnvelopeGoldenShapes(t *testing.T) {
	tests := []struct {
		name string
		env  Envelope
		want string
	}{
		{
			name: "ok",
			env:  New(StatusOK, map[string]any{"name": "recap"}, Meta{Command: "scribe list", ScribeVersion: "dev"}),
			want: `{"status":"ok","format_version":"1","data":{"name":"recap"},"meta":{"command":"scribe list","scribe_version":"dev"}}`,
		},
		{
			name: "error",
			env: Envelope{
				Status:        StatusError,
				FormatVersion: FormatVersion,
				Error:         &clierrors.Error{Code: "NOPE", Message: "not found", Retryable: false, Remediation: "retry", Resource: "recap"},
				Meta:          Meta{},
			},
			want: `{"status":"error","format_version":"1","error":{"code":"NOPE","message":"not found","retryable":false,"remediation":"retry","resource":"recap"},"meta":{}}`,
		},
		{
			name: "partial success",
			env:  New(StatusPartialSuccess, []string{"a"}, Meta{}),
			want: `{"status":"partial_success","format_version":"1","data":["a"],"meta":{}}`,
		},
		{
			name: "already installed",
			env:  New(StatusAlreadyInstalled, map[string]any{"name": "recap"}, Meta{}),
			want: `{"status":"already_installed","format_version":"1","data":{"name":"recap"},"meta":{}}`,
		},
		{
			name: "no change",
			env:  New(StatusNoChange, nil, Meta{}),
			want: `{"status":"no_change","format_version":"1","meta":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.env)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("json = %s, want %s", got, tt.want)
			}
		})
	}
}
