package a1_test

import (
	"testing"

	"github.com/amberpixels/a1"
)

func TestBudgetResolve(t *testing.T) {
	b := a1.Budget{Default: 1, Min: 1, Max: 30}
	for configured, want := range map[float64]float64{
		0:    1, // unset → default
		-5:   1, // bogus → default
		0.5:  1, // below range → default
		1:    1, // range edge kept
		12.5: 12.5,
		30:   30, // range edge kept
		31:   1,  // above range → default
	} {
		if got := b.Resolve(configured); got != want {
			t.Errorf("Resolve(%v) = %v, want %v", configured, got, want)
		}
	}
}

func TestCost(t *testing.T) {
	for _, tc := range []struct {
		model   string
		in, out int64
		want    float64
	}{
		{"claude-haiku-4-5", 1_000_000, 1_000_000, 6},       // $1 + $5
		{"claude-sonnet-4-6", 1_000_000, 0, 3},              // $3 in
		{"claude-opus-4-8", 0, 1_000_000, 25},               // $25 out
		{"claude-fable-5", 1_000_000, 0, 10},                // $10 in
		{"some-future-model", 1_000_000, 0, 10},             // unknown → top tier
		{"claude-haiku-4-5", 812, 96, (812*1 + 96*5) / 1e6}, // small-call arithmetic
	} {
		if got := a1.Cost(tc.model, tc.in, tc.out); got != tc.want {
			t.Errorf("Cost(%q, %d, %d) = %v, want %v", tc.model, tc.in, tc.out, got, tc.want)
		}
	}
}

func TestMetaCarriesCost(t *testing.T) {
	c, _ := newTestClient(t, nil, message("hi", "end_turn"))
	_, meta, err := c.Text(t.Context(), a1.Request{Task: "unit", Model: "m", Prompt: "p"})
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	// The scripted response reports model claude-haiku-4-5 with 100 in / 25 out.
	if want := a1.Cost("claude-haiku-4-5", 100, 25); meta.CostUSD != want {
		t.Errorf("meta.CostUSD = %v, want %v", meta.CostUSD, want)
	}
}
