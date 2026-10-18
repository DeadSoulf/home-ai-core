package updater

import (
	"io"
	"time"
)

// downloadProgressWriter counts bytes written to both the staged file and hash.
// Throttled reporting keeps state-file writes low during large downloads.
type downloadProgressWriter struct {
	writer     io.Writer
	progress   func(int64)
	written    int64
	lastReport time.Time
}

func (w *downloadProgressWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.written += int64(n)
	if w.progress != nil && (w.lastReport.IsZero() || time.Since(w.lastReport) >= 500*time.Millisecond) {
		w.progress(w.written)
		w.lastReport = time.Now()
	}
	return n, err
}

func (w *downloadProgressWriter) finish() {
	if w.progress != nil {
		w.progress(w.written)
	}
}
