package errors

import (
	stderrors "errors"
)

const (
	ExitOK = iota
	ExitGeneral
	ExitUsage
	ExitNotFound
	ExitPerm
	ExitConflict
	ExitNetwork
	ExitUnavailable
	ExitValid
	ExitCanceled
	ExitPartial
)

type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Retryable   bool   `json:"retryable"`
	Remediation string `json:"remediation,omitempty"`
	Resource    string `json:"resource,omitempty"`
	Exit        int    `json:"-"`
	err         error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.err != nil {
		return e.err.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

type Option func(*Error)

func WithMessage(message string) Option {
	return func(e *Error) {
		e.Message = message
	}
}

func WithRetryable(retryable bool) Option {
	return func(e *Error) {
		e.Retryable = retryable
	}
}

func WithRemediation(remediation string) Option {
	return func(e *Error) {
		e.Remediation = remediation
	}
}

func WithResource(resource string) Option {
	return func(e *Error) {
		e.Resource = resource
	}
}

func Wrap(err error, code string, exit int, opts ...Option) error {
	if err == nil {
		return nil
	}
	ce := &Error{
		Code:    code,
		Message: err.Error(),
		Exit:    exit,
		err:     err,
	}
	for _, opt := range opts {
		opt(ce)
	}
	return ce
}

func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ce *Error
	if stderrors.As(err, &ce) && ce.Exit != 0 {
		return ce.Exit
	}
	return ExitGeneral
}
