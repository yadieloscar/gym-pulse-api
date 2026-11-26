package model

import "testing"

func TestExerciseNoteKey(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantName string
		wantKey  string
	}{
		{name: "plain", in: "Leg press", wantName: "Leg press", wantKey: "leg press"},
		{name: "case", in: "LEG Press", wantName: "LEG Press", wantKey: "leg press"},
		{name: "outer and inner spacing", in: "  Leg \t press\n", wantName: "Leg press", wantKey: "leg press"},
		{name: "punctuation is kept", in: "Push-up (wide)", wantName: "Push-up (wide)", wantKey: "push-up (wide)"},
		{name: "non-ASCII", in: "Élévation Latérale", wantName: "Élévation Latérale", wantKey: "élévation latérale"},
		{name: "only spaces", in: " \t ", wantName: "", wantKey: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanExerciseName(tt.in); got != tt.wantName {
				t.Errorf("CleanExerciseName(%q) = %q, want %q", tt.in, got, tt.wantName)
			}
			if got := ExerciseNoteKey(tt.in); got != tt.wantKey {
				t.Errorf("ExerciseNoteKey(%q) = %q, want %q", tt.in, got, tt.wantKey)
			}
		})
	}
}
