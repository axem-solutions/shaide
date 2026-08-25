package writer

import (
	"strings"
	"sync"

	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

type ChanWriter struct {
	mu       sync.Mutex
	ch       chan<- messages.LogEntry
	buf      strings.Builder
	liveLine bool
	inPlace  bool
	lastLine string
}

func NewChanWriter(ch chan<- messages.LogEntry) *ChanWriter {
	return &ChanWriter{ch: ch}
}

func (w *ChanWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, b := range p {
		switch b {
		case '\r':
			w.flush(w.liveLine)
			w.inPlace = true

		case '\n':
			w.flush(w.liveLine)
			w.inPlace = false
			w.liveLine = false

		default:
			w.buf.WriteByte(b)
		}
	}

	if w.inPlace && w.buf.Len() > 0 {
		w.emit(w.buf.String(), w.liveLine)
		w.buf.Reset()
		w.liveLine = true
	}

	return len(p), nil
}

func (w *ChanWriter) flush(replace bool) {
	if w.buf.Len() == 0 {
		return
	}

	w.emit(w.buf.String(), replace)
	w.buf.Reset()
	w.liveLine = true
}

func (w *ChanWriter) emit(line string, replace bool) {
	line = strings.TrimSpace(stripANSIEscapes(line))
	if line == "" || replace && line == w.lastLine {
		return
	}

	w.lastLine = line
	w.ch <- messages.LogEntry{
		Line:    line,
		Replace: replace,
	}
}

func stripANSIEscapes(s string) string {
	var out strings.Builder
	out.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			out.WriteByte(s[i])
			continue
		}

		i++
		if i >= len(s) {
			break
		}

		if s[i] != '[' {
			continue
		}

		for i++; i < len(s) && (s[i] < 0x40 || s[i] > 0x7e); i++ {
		}
	}

	return out.String()
}
