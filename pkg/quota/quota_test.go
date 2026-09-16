package quota

import (
	"testing"
	"time"
)

func TestTavilyItemPrefersAccountPlanTotals(t *testing.T) {
	var body tavilyUsageResponse
	body.Key.Usage = 0
	body.Key.Limit = 0
	body.Account.CurrentPlan = "Researcher"
	body.Account.PlanUsage = 323
	body.Account.PlanLimit = 1000

	item := tavilyItem(body, time.Unix(0, 0).UTC())
	if item.Used == nil || *item.Used != 323 {
		t.Fatalf("used = %v, want 323", item.Used)
	}
	if item.Limit == nil || *item.Limit != 1000 {
		t.Fatalf("limit = %v, want 1000", item.Limit)
	}
	if item.Remaining == nil || *item.Remaining != 677 {
		t.Fatalf("remaining = %v, want 677", item.Remaining)
	}
}

func TestTavilyItemFallsBackToKeyTotals(t *testing.T) {
	var body tavilyUsageResponse
	body.Key.Usage = 150
	body.Key.Limit = 1000

	item := tavilyItem(body, time.Unix(0, 0).UTC())
	if item.Used == nil || *item.Used != 150 {
		t.Fatalf("used = %v, want 150", item.Used)
	}
	if item.Limit == nil || *item.Limit != 1000 {
		t.Fatalf("limit = %v, want 1000", item.Limit)
	}
	if item.Remaining == nil || *item.Remaining != 850 {
		t.Fatalf("remaining = %v, want 850", item.Remaining)
	}
}
