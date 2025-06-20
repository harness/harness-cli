package progress

import (
	"fmt"
	"io"

	"github.com/pterm/pterm"
	"harness/util/common"
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

type progressReadCloser struct {
	io.Reader       // embedded -- satisfies Read
	closeUnderlying io.Closer
	bar             *pterm.ProgressbarPrinter
}

func (p *progressReadCloser) Close() error {
	// Always stop the bar.
	p.bar.Stop()

	// Close the wrapped reader if it has a Close method.
	if p.closeUnderlying != nil {
		pterm.Success.Println(p.bar.Title)
		return p.closeUnderlying.Close()
	}
	return nil
}

// Reader returns an io.ReadCloser that copies every byte read into a progress bar.
func ReadCloser(contentLength int64, r io.Reader, saveFilename string) io.ReadCloser {
	title := fmt.Sprintf("%s (%s)", saveFilename, common.GetSize(contentLength))
	bar := pterm.DefaultProgressbar.
		WithTitle(title).
		WithRemoveWhenDone(false)

	if contentLength > 0 {
		bar = bar.WithTotal(int(contentLength))
	}

	pb, _ := bar.Start()

	// Build the TeeReader that feeds the bar.
	tee := io.TeeReader(r, &BarWriter{pb})

	// If the incoming reader also has a Close method, remember it.
	var closer io.Closer
	if c, ok := r.(io.Closer); ok {
		closer = c
	}

	return &progressReadCloser{
		Reader:          tee,
		closeUnderlying: closer,
		bar:             pb,
	}
}
