package types

import (
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
)

// ErrorHook is a zerolog hook that prints errors using pterm
type ErrorHook struct{}

// Run implements the zerolog.Hook interface
func (h ErrorHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if level == zerolog.ErrorLevel {
		pterm.Error.Println(msg)
	}
}
