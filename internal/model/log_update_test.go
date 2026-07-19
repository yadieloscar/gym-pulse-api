package model

import (
	"encoding/json"
	"testing"
)

func TestUpdateDayLogRequest_FieldPresence(t *testing.T) {
	tests := []struct {
		name                      string
		body                      string
		overrides, setLogs, notes bool
		overridesLen, setLogsLen  int
		notesValue                *string
	}{
		{name: "omitted fields preserve", body: `{}`},
		{name: "empty arrays clear", body: `{"overrides":[],"set_logs":[]}`, overrides: true, setLogs: true},
		{name: "null collections clear", body: `{"overrides":null,"set_logs":null}`, overrides: true, setLogs: true},
		{name: "populated collections replace", body: `{"overrides":[{"exercise_id":"00000000-0000-0000-0000-000000000001"}],"set_logs":[{"exercise_id":"00000000-0000-0000-0000-000000000002","set_index":1}]}`, overrides: true, setLogs: true, overridesLen: 1, setLogsLen: 1},
		{name: "null notes clear", body: `{"session_notes":null}`, notes: true},
		{name: "empty notes clear", body: `{"session_notes":""}`, notes: true, notesValue: stringPointer("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got UpdateDayLogRequest
			if err := json.Unmarshal([]byte(tt.body), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.OverridesSet != tt.overrides || got.SetLogsSet != tt.setLogs || got.SessionNotesSet != tt.notes {
				t.Fatalf("presence = overrides:%v set_logs:%v notes:%v", got.OverridesSet, got.SetLogsSet, got.SessionNotesSet)
			}
			if len(got.Overrides) != tt.overridesLen || len(got.SetLogs) != tt.setLogsLen {
				t.Fatalf("collection lengths = overrides:%d set_logs:%d", len(got.Overrides), len(got.SetLogs))
			}
			if tt.notesValue != nil && (got.SessionNotes == nil || *got.SessionNotes != *tt.notesValue) {
				t.Fatalf("session notes = %v", got.SessionNotes)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }
