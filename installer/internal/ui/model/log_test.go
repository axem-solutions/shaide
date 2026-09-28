package model

import (
	"errors"
	"strings"
	"testing"
)

// The operator knows the storage directory by its host path, so a saved log is
// named relative to it rather than by its path inside the container.
func TestLogsSavedNamesThePathInTheStorageDirectory(t *testing.T) {
	m := New(nil)
	m = m.handleLogsSaved(logsSavedMsg{
		LineCount: 482,
		Path:      "/var/shaide-installer/logs/installer-logs-20260927-165610.log",
	})

	if want := "saved 482 lines to logs/installer-logs-20260927-165610.log"; m.LogSaveStatus != want {
		t.Errorf("status = %q, want %q", m.LogSaveStatus, want)
	}

	last := m.Logs[len(m.Logs)-1]
	if want := "Logs saved to logs/installer-logs-20260927-165610.log in the storage directory"; last != want {
		t.Errorf("log line = %q, want %q", last, want)
	}
	for _, line := range append(m.Logs, m.LogSaveStatus) {
		if strings.Contains(line, "/var/shaide-installer") {
			t.Errorf("%q names the container path", line)
		}
	}
}

func TestLogsSaveFailureIsReported(t *testing.T) {
	m := New(nil)
	m = m.handleLogsSaved(logsSavedMsg{Err: errors.New("disk full")})

	if !strings.HasPrefix(m.LogSaveStatus, "save failed: disk full") {
		t.Errorf("status = %q, want the failure", m.LogSaveStatus)
	}
}
