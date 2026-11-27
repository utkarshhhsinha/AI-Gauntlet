package cerebra

import (
	"testing"
)

func TestQuota_CheckTokenQuota(t *testing.T) {
	const limit int64 = 100_000

	tests := []struct {
		name          string
		used          int64
		wantMilestone QuotaMilestone
		wantExhausted bool
	}{
		{"under 50%", 30_000, MilestoneNone, false},
		{"exactly 50%", 50_000, Milestone50Percent, false},
		{"at 75%", 75_000, Milestone50Percent, false},
		{"exactly 90%", 90_000, Milestone90Percent, false},
		{"at 95%", 95_000, Milestone90Percent, false},
		{"exactly 100%", 100_000, Milestone100Percent, true},
		{"over 100% (exhausted)", 125_000, Milestone100Percent, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			check := CheckTokenQuota(tc.used, limit)
			if check.Milestone != tc.wantMilestone {
				t.Errorf("got milestone %q, want %q", check.Milestone, tc.wantMilestone)
			}
			if check.IsExhausted != tc.wantExhausted {
				t.Errorf("got isExhausted %v, want %v", check.IsExhausted, tc.wantExhausted)
			}
		})
	}
}

func TestQuota_ResolveModelTokenLimit(t *testing.T) {
	if limit := ResolveModelTokenLimit("claude-opus-4.5", 0); limit != 200_000 {
		t.Errorf("expected 200k for opus, got %d", limit)
	}
	if limit := ResolveModelTokenLimit("claude-haiku-4.5", 0); limit != 100_000 {
		t.Errorf("expected 100k for haiku, got %d", limit)
	}
	if limit := ResolveModelTokenLimit("any-model", 50_000); limit != 50_000 {
		t.Errorf("expected custom limit 50k to take precedence, got %d", limit)
	}
}
