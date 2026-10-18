package chat

import (
	tea "charm.land/bubbletea/v2"
	"time"
)

func (s *ChatSources) OpenDocument() tea.Cmd {
	record := s.Evidence.Records[s.Index]
	s.Loading, s.Err = true, ""
	return func() tea.Msg {
		body := "Saved reference: " + record.Ref + "\nRecorded: " + record.Time.Format(time.RFC3339Nano) + "\nStatus: " + record.Status + "\n\nRequest\n" + PrettySource(string(record.Input)) + "\n\nCaptured result\n" + PrettySource(record.Content)
		return ChatSourceDocumentResult{s, body, nil}
	}
}
