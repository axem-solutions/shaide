package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/events"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/model"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/writer"
)

type App struct {
	Program  *tea.Program
	Reporter *events.Reporter
	Writer   *writer.ChanWriter
	LogCh    chan messages.LogEntry
}

func New() *App {
	logCh := make(chan messages.LogEntry, 256)

	m := model.New(logCh)
	program := tea.NewProgram(m)

	return &App{
		Program:  program,
		Reporter: events.New(program),
		Writer:   writer.NewChanWriter(logCh),
		LogCh:    logCh,
	}
}

func (a *App) Run() error {
	_, err := a.Program.Run()
	return err
}

func (a *App) SendDone(err error) {
	a.Program.Send(messages.WorkflowDoneMessage{Err: err})
}

func (a *App) CloseLogs() {
	close(a.LogCh)
}
