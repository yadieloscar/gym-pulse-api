package model

import (
	"encoding/json"
	"testing"
)

func TestOptionalTextRecordsWhetherTheKeyWasSent(t *testing.T) {
	type body struct {
		Routine OptionalText `json:"routine"`
	}
	tests := []struct {
		name    string
		json    string
		present bool
		want    string
	}{
		{name: "absent", json: `{}`},
		{name: "null", json: `{"routine":null}`, present: true},
		{name: "empty", json: `{"routine":""}`, present: true},
		{name: "slug", json: `{"routine":"warmup-lower"}`, present: true, want: "warmup-lower"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got body
			if err := json.Unmarshal([]byte(tt.json), &got); err != nil {
				t.Fatal(err)
			}
			if got.Routine.Present != tt.present {
				t.Fatalf("Present = %v, want %v", got.Routine.Present, tt.present)
			}
			if (got.Routine.Value == nil) != (tt.want == "") || (got.Routine.Value != nil && *got.Routine.Value != tt.want) {
				t.Fatalf("Value = %v, want %q", got.Routine.Value, tt.want)
			}
		})
	}

	if err := json.Unmarshal([]byte(`{"routine":7}`), &body{}); err == nil {
		t.Fatal("a number was accepted as a routine id")
	}
	out, err := json.Marshal(body{Routine: OptionalText{Present: true, Value: nil}})
	if err != nil || string(out) != `{"routine":null}` {
		t.Fatalf("marshal = %s, %v", out, err)
	}
}
