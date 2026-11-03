package engine

import (
	"fmt"
	"strings"
)

const maxRationaleRunes = 500

var weekdayNames = map[int]string{1: "Monday", 2: "Tuesday", 3: "Wednesday", 4: "Thursday", 5: "Friday", 6: "Saturday", 7: "Sunday"}

// Rationale writes at most four plain sentences from the engine's choices and
// never mentions outcomes, diet, or medical matters (spec 016 FR-009).
func Rationale(x Explanation) string {
	var parts []string
	days := make([]string, 0, len(x.Days))
	for _, d := range x.Days {
		days = append(days, weekdayNames[d])
	}
	parts = append(parts, fmt.Sprintf("%s on %s.", capitalize(x.Split), joinWords(days)))
	if len(x.MainLifts) > 0 {
		parts = append(parts, fmt.Sprintf("Built around %s.", joinWords(lower(x.MainLifts))))
	}
	for _, note := range []string{x.SecondaryNote, x.FocusNote, x.EquipmentNote, x.BeginnerNote} {
		if note != "" {
			parts = append(parts, note)
		}
	}
	if len(x.Honoured) > 0 {
		parts = append(parts, fmt.Sprintf("No %s, as you asked.", joinWords(x.Honoured)))
	}
	if len(x.Dropped) > 0 {
		parts = append(parts, fmt.Sprintf("Could not fit %s with your equipment and limits.", joinWords(x.Dropped)))
	}
	text := strings.Join(parts, " ")
	if runes := []rune(text); len(runes) > maxRationaleRunes {
		text = strings.TrimSpace(string(runes[:maxRationaleRunes-1])) + "…"
	}
	return text
}

func joinWords(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}

func lower(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = strings.ToLower(s)
	}
	return out
}
