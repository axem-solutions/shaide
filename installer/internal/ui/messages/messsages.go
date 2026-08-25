package messages

import tea "charm.land/bubbletea/v2"

type LogMessage struct {
	Entries []LogEntry
	OK      bool
}

type LogEntry struct {
	Line    string
	Replace bool
}

type WorkflowDoneMessage struct {
	Err error
}

type PromptInputMessage struct {
	Title        string
	Placeholder  string
	DefaultValue string
	ReplyCh      chan<- PromptReply
}

type PromptSelectMessage struct {
	Title   string
	Current string
	Options []string
	ReplyCh chan<- PromptReply
}

type ModelProgressMessage struct {
	ID         string
	Bytes      int64
	TotalBytes int64
	Files      int
	TotalFiles int
	Percent    int
	Done       bool
}

type PromptMultiSelectMessage struct {
	Title   string
	Options []string
	ReplyCh chan<- PromptReply
}

type PromptReply struct {
	Values []string
	Err    error
}

func WaitForLog(ch <-chan LogEntry) tea.Cmd {
	return func() tea.Msg {
		entry, ok := <-ch
		if !ok {
			return LogMessage{OK: false}
		}

		entries := []LogEntry{entry}
		for len(entries) < 64 {
			select {
			case entry, ok := <-ch:
				if !ok {
					return LogMessage{
						Entries: entries,
						OK:      true,
					}
				}
				entries = append(entries, entry)
			default:
				return LogMessage{
					Entries: entries,
					OK:      true,
				}
			}
		}

		return LogMessage{
			Entries: entries,
			OK:      true,
		}
	}
}
