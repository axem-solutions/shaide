package events

import (
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/axem-solutions/ai_platform/installer/internal/ui/messages"
	"github.com/axem-solutions/ai_platform/installer/internal/workflow/core"
)

var ErrPromptCancelled = errors.New("prompt cancelled")

type Reporter struct {
	program *tea.Program
}

func New(program *tea.Program) *Reporter {
	return &Reporter{
		program: program,
	}
}

func (p *Reporter) Input(title string, placeholder string, defaultValue string) (string, error) {
	replyCh := make(chan messages.PromptReply, 1)

	p.program.Send(messages.PromptInputMessage{
		Title:        title,
		Placeholder:  placeholder,
		DefaultValue: defaultValue,
		ReplyCh:      replyCh,
	})

	reply := <-replyCh

	return firstPromptValue(reply)
}

func (p *Reporter) Select(title string, current string, options []string) (string, error) {
	replyCh := make(chan messages.PromptReply, 1)

	p.program.Send(messages.PromptSelectMessage{
		Title:   title,
		Current: current,
		Options: options,
		ReplyCh: replyCh,
	})

	reply := <-replyCh
	return firstPromptValue(reply)
}

func (p *Reporter) MultiSelect(title string, options []string) ([]string, error) {
	replyCh := make(chan messages.PromptReply, 1)

	p.program.Send(messages.PromptMultiSelectMessage{
		Title: title,

		Options: options,
		ReplyCh: replyCh,
	})

	reply := <-replyCh
	return reply.Values, reply.Err
}

func (r *Reporter) ProgressModel(progress core.ModelProgress) {
	r.program.Send(messages.ModelProgressMessage{
		ID:         progress.ID,
		Bytes:      progress.Bytes,
		TotalBytes: progress.TotalBytes,
		Files:      progress.Files,
		TotalFiles: progress.TotalFiles,
		Percent:    progress.Percent,
		Done:       progress.Done,
	})
}

func firstPromptValue(reply messages.PromptReply) (string, error) {
	if reply.Err != nil {
		return "", reply.Err
	}
	if len(reply.Values) == 0 {
		return "", errors.New("prompt reply contained no value")
	}
	return reply.Values[0], nil
}
