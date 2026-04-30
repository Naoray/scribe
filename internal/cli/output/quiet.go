package output

import (
	"fmt"
	"io"

	clierrors "github.com/Naoray/scribe/internal/cli/errors"
)

type quietRenderer struct {
	baseRenderer
	out    io.Writer
	errOut io.Writer
}

func newQuietRenderer(out, errOut io.Writer) *quietRenderer {
	return &quietRenderer{
		baseRenderer: newBaseRenderer(),
		out:          out,
		errOut:       errOut,
	}
}

func (r *quietRenderer) Result(data any) error {
	if data == nil {
		return nil
	}
	_, err := fmt.Fprintln(r.out, data)
	return err
}

func (r *quietRenderer) Error(err *clierrors.Error) error {
	if err == nil {
		return nil
	}
	_, writeErr := fmt.Fprintln(r.errOut, err.Error())
	return writeErr
}

func (r *quietRenderer) Progress(string) {}

func (r *quietRenderer) Flush() error { return nil }
