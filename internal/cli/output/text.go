package output

import (
	"encoding/json"
	"fmt"
	"io"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

type textRenderer struct {
	baseRenderer
	out     io.Writer
	errOut  io.Writer
	color   bool
	flushed bool
}

func newTextRenderer(out, errOut io.Writer, color bool) *textRenderer {
	return &textRenderer{
		baseRenderer: newBaseRenderer(),
		out:          out,
		errOut:       errOut,
		color:        color,
	}
}

func (r *textRenderer) Result(data any) error {
	switch v := data.(type) {
	case nil:
		return nil
	case string:
		_, err := fmt.Fprintln(r.out, v)
		return err
	default:
		bytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(r.out, string(bytes))
		return err
	}
}

func (r *textRenderer) Error(err *clierrors.Error) error {
	if err == nil {
		return nil
	}
	if err.Code != "" {
		_, _ = fmt.Fprintf(r.errOut, "error[%s]: %s\n", err.Code, err.Error())
	} else {
		_, _ = fmt.Fprintf(r.errOut, "error: %s\n", err.Error())
	}
	if err.Remediation != "" {
		_, _ = fmt.Fprintf(r.errOut, "remediation: %s\n", err.Remediation)
	}
	return nil
}

func (r *textRenderer) Progress(msg string) {
	if msg != "" {
		fmt.Fprintln(r.errOut, msg)
	}
}

func (r *textRenderer) Flush() error {
	r.flushed = true
	return nil
}
