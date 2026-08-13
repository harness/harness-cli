package migratable

import (
	"os"
	"sync/atomic"
	"time"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// stageHeartbeatInterval is how often a non-interactive stage reprints its
// liveness line.
const stageHeartbeatInterval = 15 * time.Second

// spinnerBusy serialises the animated spinner. Registry jobs run concurrently
// and two spinners redrawing the same terminal line interleave into garbage, so
// the first claimant animates and later stages fall back to heartbeat lines.
var spinnerBusy atomic.Bool

// stageProgress reports liveness for a long-running enumeration call that has
// no incremental progress to report. On a terminal it animates a pterm spinner
// with an elapsed timer; when stdout is redirected (log file, CI) it prints a
// heartbeat line every stageHeartbeatInterval so a slow registry is
// distinguishable from a hung one.
type stageProgress struct {
	text      string
	startedAt time.Time

	spinner *pterm.SpinnerPrinter

	stop chan struct{}
	done chan struct{}
}

func startStage(text string) *stageProgress {
	s := &stageProgress{text: text, startedAt: time.Now()}

	if isInteractiveTerminal() && spinnerBusy.CompareAndSwap(false, true) {
		spinner, err := pterm.DefaultSpinner.
			WithRemoveWhenDone(true).
			WithShowTimer(true).
			Start(text)
		if err == nil {
			s.spinner = spinner
			return s
		}
		spinnerBusy.Store(false)
	}

	pterm.Info.Println(text)
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	go s.heartbeat()
	return s
}

// success ends the stage with a success line carrying the elapsed time.
func (s *stageProgress) success(msg string) {
	s.halt()
	pterm.Success.Println(s.withElapsed(msg))
}

// warn ends the stage with a warning line, for a degraded but non-fatal outcome.
func (s *stageProgress) warn(msg string) {
	s.halt()
	pterm.Warning.Println(s.withElapsed(msg))
}

// fail ends the stage with an error line.
func (s *stageProgress) fail(msg string) {
	s.halt()
	pterm.Error.Println(s.withElapsed(msg))
}

func (s *stageProgress) heartbeat() {
	defer close(s.done)

	ticker := time.NewTicker(stageHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			pterm.Info.Println(s.withElapsed(s.text + " — still working"))
		}
	}
}

// halt stops the animation or heartbeat. Safe to call more than once.
func (s *stageProgress) halt() {
	if s.spinner != nil {
		_ = s.spinner.Stop()
		s.spinner = nil
		spinnerBusy.Store(false)
		return
	}
	if s.stop != nil {
		close(s.stop)
		<-s.done
		s.stop = nil
	}
}

func (s *stageProgress) withElapsed(msg string) string {
	return msg + " (" + time.Since(s.startedAt).Round(time.Second).String() + ")"
}

func isInteractiveTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
