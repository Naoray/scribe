package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Naoray/scribe/internal/cli/envelope"
	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

type jsonRenderer struct {
	baseRenderer
	out     io.Writer
	errOut  io.Writer
	data    any
	err     *clierrors.Error
	hasData bool
	flushed bool
}

func newJSONRenderer(out, errOut io.Writer) *jsonRenderer {
	return &jsonRenderer{
		baseRenderer: newBaseRenderer(),
		out:          out,
		errOut:       errOut,
	}
}

func (r *jsonRenderer) Result(data any) error {
	r.data = data
	r.hasData = true
	return nil
}

func (r *jsonRenderer) Error(err *clierrors.Error) error {
	r.err = err
	r.SetStatus(envelope.StatusError)
	return r.flushTo(r.out)
}

func (r *jsonRenderer) Progress(msg string) {
	if msg == "" {
		return
	}
	fmt.Fprintln(r.errOut, msg)
}

func (r *jsonRenderer) Flush() error {
	return r.flushTo(r.out)
}

func (r *jsonRenderer) flushTo(w io.Writer) error {
	if r.flushed {
		return nil
	}
	if !r.hasData && r.err == nil {
		return nil
	}
	r.flushed = true

	env := envelope.Envelope{
		Status:        r.status,
		FormatVersion: envelope.FormatVersion,
		Data:          r.data,
		Error:         r.err,
		Meta:          r.envelopeMeta(),
	}
	if r.err != nil {
		env.Status = envelope.StatusError
	}
	enc := json.NewEncoder(w)
	return enc.Encode(env)
}
