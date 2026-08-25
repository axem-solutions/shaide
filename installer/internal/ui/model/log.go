package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/config/paths"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

const maxLogLines = 5000

type logsSavedMsg struct {
	LineCount int
	Path      string
	Err       error
}

func (m *Model) appendLog(entry messages.LogEntry) {
	line := strings.TrimSpace(entry.Line)
	if line == "" {
		return
	}

	m.LogSaveStatus = ""

	if entry.Replace && len(m.Logs) > 0 {
		m.Logs[len(m.Logs)-1] = line
	} else {
		m.Logs = append(m.Logs, line)
	}

	if len(m.Logs) > maxLogLines {
		m.Logs = m.Logs[len(m.Logs)-maxLogLines:]
	}

	m.LogViewport.SetContent(strings.Join(m.Logs, "\n"))
	m.LogViewport.GotoBottom()
}

func (m Model) saveLogsToStorage() (tea.Model, tea.Cmd) {
	if len(m.Logs) == 0 {
		m.LogSaveStatus = "no logs to save"
		return m, nil
	}

	lineCount := len(m.Logs)
	text := strings.Join(m.Logs, "\n")
	m.LogSaveStatus = "saving..."

	return m, func() tea.Msg {
		path, err := writeLogsToStorage(text)
		return logsSavedMsg{
			LineCount: lineCount,
			Path:      path,
			Err:       err,
		}
	}
}

func (m Model) handleLogsSaved(msg logsSavedMsg) Model {
	if msg.Err != nil {
		m.LogSaveStatus = "save failed: " + compactMessage(msg.Err.Error())
		return m
	}

	m.LogSaveStatus = fmt.Sprintf("saved %s to %s", pluralize(msg.LineCount, "line", "lines"), msg.Path)
	return m
}

func writeLogsToStorage(text string) (string, error) {
	logDir := paths.DefaultPaths().Logs
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", fmt.Errorf("create log directory %s: %w", logDir, err)
	}

	name := "installer-logs-" + time.Now().Format("20060102-150405") + ".log"
	path := filepath.Join(logDir, name)
	content := strings.TrimRight(text, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return path, fmt.Errorf("write log file %s: %w", path, err)
	}

	return path, nil
}

func pluralize(count int, singular string, plural string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, singular)
	}

	return fmt.Sprintf("%d %s", count, plural)
}

func workflowErrorLogLine(err error) string {
	return errorStyle.Render("failed: " + compactMessage(err.Error()))
}

func compactMessage(message string) string {
	return strings.Join(strings.Fields(message), " ")
}
