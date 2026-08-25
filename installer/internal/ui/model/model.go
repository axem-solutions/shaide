package model

import (
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
)

type Mode int

const (
	ModeIdle Mode = iota
	ModeInput
	ModeSelect
	ModeMultiSelect
	ModeDone
)

type Model struct {
	Mode Mode

	HasDarkBackground bool

	LogCh         <-chan messages.LogEntry
	Logs          []string
	LogSaveStatus string

	ReplyCh chan<- messages.PromptReply

	LogViewport viewport.Model
	Input       textinput.Model
	List        list.Model

	Width  int
	Height int

	PromptTitle       string
	PromptPlaceholder string
	SelectWidth       int

	Progress        progress.Model
	ProgressActive  bool
	ProgressID      string
	ProgressPercent int
	ProgressBytes   int64
	ProgressTotal   int64

	Done bool
	Err  error
}

func New(logCh <-chan messages.LogEntry) Model {
	input := textinput.New()
	input.Placeholder = "type value..."
	input.Focus()

	return Model{
		Mode:              ModeIdle,
		HasDarkBackground: true,
		LogCh:             logCh,
		Logs:              make([]string, 0, 256),
		Input:             input,
		Progress:          progress.New(progress.WithoutPercentage()),
		LogViewport:       newLogViewport(80, 20),
	}
}

func newLogViewport(width int, height int) viewport.Model {
	logViewport := viewport.New(
		viewport.WithWidth(width),
		viewport.WithHeight(height),
	)
	logViewport.SoftWrap = false
	logViewport.SetHorizontalStep(12)
	logViewport.FillHeight = true

	return logViewport
}
