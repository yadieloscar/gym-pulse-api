package model

import "testing"

func TestTrainingExposureQualifies(t *testing.T) {
	baseline := NextMorningBaseline
	above := NextMorningAboveBaseline
	for _, tc := range []struct {
		name     string
		outcome  string
		response *string
		want     bool
	}{
		{"completed and baseline", SessionCompletedAsPlanned, &baseline, true},
		{"completed and pending", SessionCompletedAsPlanned, nil, false},
		{"completed and above", SessionCompletedAsPlanned, &above, false},
		{"modified and baseline", SessionModified, &baseline, false},
		{"stopped and baseline", SessionStopped, &baseline, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrainingExposureQualifies(tc.outcome, tc.response); got != tc.want {
				t.Fatalf("TrainingExposureQualifies() = %v, want %v", got, tc.want)
			}
		})
	}
}
