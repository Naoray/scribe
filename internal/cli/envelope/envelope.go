package envelope

import clierrors "github.com/Naoray/scribe/internal/cli/errors"

const FormatVersion = "1"

type Status string

const (
	StatusOK               Status = "ok"
	StatusPartialSuccess   Status = "partial_success"
	StatusAlreadyInstalled Status = "already_installed"
	StatusNoChange         Status = "no_change"
	StatusError            Status = "error"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusOK, StatusPartialSuccess, StatusAlreadyInstalled, StatusNoChange, StatusError:
		return true
	default:
		return false
	}
}

type Meta struct {
	DurationMS    int64  `json:"duration_ms,omitempty"`
	BootstrapMS   int64  `json:"bootstrap_ms,omitempty"`
	Command       string `json:"command,omitempty"`
	ScribeVersion string `json:"scribe_version,omitempty"`
}

type Envelope struct {
	Status        Status           `json:"status"`
	FormatVersion string           `json:"format_version"`
	Data          any              `json:"data,omitempty"`
	Error         *clierrors.Error `json:"error,omitempty"`
	Meta          Meta             `json:"meta"`
}

func New(status Status, data any, meta Meta) Envelope {
	if !status.IsValid() {
		status = StatusError
	}
	return Envelope{
		Status:        status,
		FormatVersion: FormatVersion,
		Data:          data,
		Meta:          meta,
	}
}
