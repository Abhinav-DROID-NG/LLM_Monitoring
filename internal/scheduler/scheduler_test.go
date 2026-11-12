package scheduler

import (
	"testing"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

func TestExpandMonitoringCadence(t *testing.T) {
	suite := canary.Suite{Tasks: []canary.Task{
		{ID: "easy", Tier: "easy"}, {ID: "general", Tier: "general"}, {ID: "expensive", Tier: "expensive"},
	}}
	if got := len(Expand(suite, 1, Monitoring).Tasks); got != 16 {
		t.Fatalf("cycle 1 tasks = %d, want 16", got)
	}
	if got := len(Expand(suite, 4, Monitoring).Tasks); got != 19 {
		t.Fatalf("cycle 4 tasks = %d, want 19", got)
	}
}
