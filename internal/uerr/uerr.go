// Package uerr is for errors the user is meant to read. Each one carries a
// stable code and the values that went into it, so the web interface can
// show it in the visitor's language, while Error() stays plain English for
// the CLI and the JSON API.
package uerr

import (
	"errors"
	"fmt"
)

// Error is a validation failure with a translatable identity. Code names
// the catalogue entry (error.<code> in the js-app namespace) and Args holds
// its placeholders.
type Error struct {
	Code string
	Args map[string]any
	text string
}

// New builds an Error. args are name/value pairs, the way i18n.T takes them:
// New("group.tooLong", "group name cannot be longer than 32 characters", "max", 32).
func New(code, text string, args ...any) *Error {
	e := &Error{Code: code, text: text}
	if len(args) > 0 {
		e.Args = make(map[string]any, len(args)/2)
		for i := 0; i+1 < len(args); i += 2 {
			if k, ok := args[i].(string); ok {
				e.Args[k] = args[i+1]
			}
		}
	}
	return e
}

// Newf is New with the English text built by fmt.Sprintf from the same
// values, for the common case where the message and the placeholders agree:
// Newf("location.exists", "location %q already exists", "name", name).
func Newf(code, format string, args ...any) *Error {
	vals := make([]any, 0, len(args)/2)
	for i := 1; i < len(args); i += 2 {
		vals = append(vals, args[i])
	}
	return New(code, fmt.Sprintf(format, vals...), args...)
}

func (e *Error) Error() string { return e.text }

// Of returns the Error inside err, if there is one.
func Of(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
