package logger

import (
	"io"
	"log"
	"time"
)

type Logger struct {
	out *log.Logger
}

func NewWithWriter(w io.Writer) *Logger {
	return &Logger{
		out: log.New(w, "", log.LstdFlags),
	}
}

func (l *Logger) Writer() io.Writer {
	return l.out.Writer()
}

func (l *Logger) Printf(format string, args ...any) {
	l.out.Printf(format, args...)
}

func (l *Logger) StageStart(name string, stepCount int) {
	l.Printf("Stage: %s(%d %s)", name, stepCount, pluralize(stepCount, "step", "steps"))
}

func (l *Logger) StageDone(name string, elapsed time.Duration) {
	l.Printf("Stage complete: %s (%s)", name, formatElapsed(elapsed))
	l.Printf("=======")
}

func (l *Logger) StepStart(index, total int, name string) {
	l.Printf("Step %d/%d: %s", index, total, name)
}

func (l *Logger) StepDone(elapsed time.Duration) {
	l.Printf("Done in: %s", formatElapsed(elapsed))
}

func (l *Logger) Detailf(format string, args ...any) {
	l.Printf(format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.Printf("[error]"+format, args...)
}

func formatElapsed(elapsed time.Duration) string {
	switch {
	case elapsed < time.Millisecond:
		return elapsed.Round(time.Microsecond).String()
	default:
		return elapsed.Round(time.Millisecond).String()
	}
}

func pluralize(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}

	return plural
}
