package lane

import (
	"testing"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
)

func TestClassifier_Classify(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name             string
		slideWindowWidth int32
		wantLane         domain.Lane
	}{
		// Strict lane cases (<=150 seconds = 2.5 minutes one-sided)
		{
			name:             "SlideWindowWidth=0 should be Strict",
			slideWindowWidth: 0,
			wantLane:         domain.LaneStrict,
		},
		{
			name:             "SlideWindowWidth=1 should be Strict",
			slideWindowWidth: 1,
			wantLane:         domain.LaneStrict,
		},
		{
			name:             "SlideWindowWidth=60 (1 min) should be Strict",
			slideWindowWidth: 60,
			wantLane:         domain.LaneStrict,
		},
		{
			name:             "SlideWindowWidth=120 (2 min, short/scheduled TargetAt) should be Strict",
			slideWindowWidth: 120,
			wantLane:         domain.LaneStrict,
		},
		{
			name:             "SlideWindowWidth=150 (threshold) should be Strict",
			slideWindowWidth: 150,
			wantLane:         domain.LaneStrict,
		},
		// Loose lane cases (>150 seconds)
		{
			name:             "SlideWindowWidth=151 should be Loose",
			slideWindowWidth: 151,
			wantLane:         domain.LaneLoose,
		},
		{
			name:             "SlideWindowWidth=180 (3 min) should be Loose",
			slideWindowWidth: 180,
			wantLane:         domain.LaneLoose,
		},
		{
			name:             "SlideWindowWidth=300 (5 min, near/relaxed TargetAt) should be Loose",
			slideWindowWidth: 300,
			wantLane:         domain.LaneLoose,
		},
		{
			name:             "SlideWindowWidth=600 (10 min, max intermediate) should be Loose",
			slideWindowWidth: 600,
			wantLane:         domain.LaneLoose,
		},
		{
			name:             "SlideWindowWidth=1800 (30 min) should be Loose",
			slideWindowWidth: 1800,
			wantLane:         domain.LaneLoose,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remind := timemgmt.RemindResponse{
				ID:               "remind-1",
				TaskID:           "task-1",
				SlideWindowWidth: tt.slideWindowWidth,
			}

			got := classifier.Classify(remind)

			if got != tt.wantLane {
				t.Errorf("Classify() = %v, want %v", got, tt.wantLane)
			}
		})
	}
}

func TestClassifier_ClassifyWithThreshold(t *testing.T) {
	classifier := NewClassifier()

	// Verify the threshold constant value is now 150
	if StrictSlideWindowThreshold != 150 {
		t.Errorf("StrictSlideWindowThreshold = %d, want 150", StrictSlideWindowThreshold)
	}

	// Boundary test: exactly at threshold should be Strict
	remind := timemgmt.RemindResponse{
		SlideWindowWidth: StrictSlideWindowThreshold,
	}
	if got := classifier.Classify(remind); got != domain.LaneStrict {
		t.Errorf("Classify at threshold = %v, want LaneStrict", got)
	}

	// Just above threshold should be Loose
	remind.SlideWindowWidth = StrictSlideWindowThreshold + 1
	if got := classifier.Classify(remind); got != domain.LaneLoose {
		t.Errorf("Classify above threshold = %v, want LaneLoose", got)
	}
}

// TestClassifier_IntegrationWithTimeMgmtWidths verifies classification
// aligns with the new time-mgmt window widths
func TestClassifier_IntegrationWithTimeMgmtWidths(t *testing.T) {
	classifier := NewClassifier()

	tests := []struct {
		name             string
		description      string
		slideWindowWidth int32
		wantLane         domain.Lane
	}{
		// TargetAt widths from time-mgmt
		{
			name:             "short_targetat",
			description:      "short/scheduled type TargetAt (2 min) should be Strict",
			slideWindowWidth: 120, // 2 minutes
			wantLane:         domain.LaneStrict,
		},
		{
			name:             "near_targetat",
			description:      "near/relaxed type TargetAt (5 min) should be Loose",
			slideWindowWidth: 300, // 5 minutes
			wantLane:         domain.LaneLoose,
		},
		// Intermediate widths from time-mgmt
		{
			name:             "intermediate_min",
			description:      "Intermediate with min width (1 min) should be Strict",
			slideWindowWidth: 60, // 1 minute
			wantLane:         domain.LaneStrict,
		},
		{
			name:             "intermediate_mid",
			description:      "Intermediate with mid width (3 min) should be Loose",
			slideWindowWidth: 180, // 3 minutes
			wantLane:         domain.LaneLoose,
		},
		{
			name:             "intermediate_max",
			description:      "Intermediate with max width (10 min) should be Loose",
			slideWindowWidth: 600, // 10 minutes
			wantLane:         domain.LaneLoose,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remind := timemgmt.RemindResponse{
				SlideWindowWidth: tt.slideWindowWidth,
			}

			got := classifier.Classify(remind)

			if got != tt.wantLane {
				t.Errorf("%s: Classify() = %v, want %v", tt.description, got, tt.wantLane)
			}
		})
	}
}
