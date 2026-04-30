package output

import (
	"io"

	"github.com/Naoray/scribe/internal/cli/env"
	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

type Renderer interface {
	Result(any) error
	Error(*clierrors.Error) error
	Progress(string)
	SetMeta(string, any)
	SetStatus(envelope.Status)
	Flush() error
}

func New(mode env.Mode, out, errOut io.Writer) Renderer {
	switch mode.Format {
	case env.FormatJSON:
		return newJSONRenderer(out, errOut)
	case env.FormatQuiet:
		return newQuietRenderer(out, errOut)
	default:
		return newTextRenderer(out, errOut, mode.Color)
	}
}

type baseRenderer struct {
	meta   map[string]any
	status envelope.Status
}

func newBaseRenderer() baseRenderer {
	return baseRenderer{
		meta:   map[string]any{},
		status: envelope.StatusOK,
	}
}

func (r *baseRenderer) SetMeta(k string, v any) {
	if k == "" {
		return
	}
	r.meta[k] = v
}

func (r *baseRenderer) SetStatus(status envelope.Status) {
	if status.IsValid() {
		r.status = status
	}
}

func (r *baseRenderer) envelopeMeta() envelope.Meta {
	meta := envelope.Meta{}
	if v, ok := r.meta["duration_ms"].(int64); ok {
		meta.DurationMS = v
	}
	if v, ok := r.meta["bootstrap_ms"].(int64); ok {
		meta.BootstrapMS = v
	}
	if v, ok := r.meta["command"].(string); ok {
		meta.Command = v
	}
	if v, ok := r.meta["scribe_version"].(string); ok {
		meta.ScribeVersion = v
	}
	return meta
}
