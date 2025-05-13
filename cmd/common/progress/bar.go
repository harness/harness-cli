package progress

import (
	"fmt"
	"io"

	"github.com/pterm/pterm"
	"harness/cmd/common"
)

type BarWriter struct {
	bar *pterm.ProgressbarPrinter
}

func (w *BarWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.bar.Add(n)
	return n, nil
}

func Reader(contentLength int64, reader io.Reader, saveFilename string) (io.Reader, func()) {
	title := fmt.Sprintf("%s (%s)", saveFilename, common.GetSize(contentLength))
	bar := pterm.DefaultProgressbar.
		WithTitle(title).WithRemoveWhenDone(false)

	if contentLength > 0 {
		bar = bar.WithTotal(int(contentLength))
	}

	pb, _ := bar.Start()

	r := io.TeeReader(reader, &BarWriter{pb})
	return r, func() { defer pb.Stop() }
}
