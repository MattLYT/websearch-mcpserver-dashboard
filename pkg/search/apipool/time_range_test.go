package apipool

import (
	"testing"
	"websearch/pkg/search/core"
)

type timeRangeCallRecorder struct{ rawCalls, rangeCalls, days int }

func (*timeRangeCallRecorder) Name() string                  { return "test" }
func (*timeRangeCallRecorder) Search(string) (string, error) { return "", nil }
func (*timeRangeCallRecorder) MergeContent(string, []core.SearchResult) (string, error) {
	return "", nil
}
func (r *timeRangeCallRecorder) SearchRaw(string) ([]core.SearchResult, error) {
	r.rawCalls++
	return []core.SearchResult{{Title: "default"}}, nil
}
func (r *timeRangeCallRecorder) SearchRawWithTimeRange(_ string, days int) ([]core.SearchResult, error) {
	r.rangeCalls++
	r.days = days
	return []core.SearchResult{{Title: "ranged"}}, nil
}

func TestApipoolTimeRangeZeroAndDefault(t *testing.T) {
	r := &timeRangeCallRecorder{}
	a := NewApipoolSearch("priority", NewApipoolProvider("test", r, nil))
	if _, err := a.SearchRawWithTimeRange("unlimited", 0); err != nil {
		t.Fatal(err)
	}
	if r.rangeCalls != 1 || r.rawCalls != 0 || r.days != 0 {
		t.Errorf("explicit zero: raw=%d ranged=%d days=%d", r.rawCalls, r.rangeCalls, r.days)
	}
	if _, err := a.SearchRaw("default"); err != nil {
		t.Fatal(err)
	}
	if r.rangeCalls != 1 || r.rawCalls != 1 {
		t.Errorf("default: raw=%d ranged=%d", r.rawCalls, r.rangeCalls)
	}
	if _, err := a.SearchRawWithTimeRange("week", 7); err != nil {
		t.Fatal(err)
	}
	if r.rangeCalls != 2 || r.rawCalls != 1 || r.days != 7 {
		t.Errorf("week: raw=%d ranged=%d days=%d", r.rawCalls, r.rangeCalls, r.days)
	}
}
