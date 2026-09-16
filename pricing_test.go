package a1_test

import (
	"testing"

	"github.com/amberpixels/a1"
)

func TestCost(t *testing.T) {
	c := a1.NewClient("test-key")
	for _, tc := range []struct {
		model   string
		in, out int64
		want    float64
	}{
		{"claude-haiku-4-5", 1_000_000, 1_000_000, 6},       // $1 + $5
		{"claude-sonnet-4-6", 1_000_000, 0, 3},              // the family price
		{"claude-sonnet-5", 1_000_000, 0, 2},                // a per-model entry beats it
		{"claude-opus-4-8", 0, 1_000_000, 25},               // $25 out
		{"claude-fable-5", 1_000_000, 0, 10},                // $10 in
		{"some-future-model", 1_000_000, 0, 10},             // unknown → top tier
		{"claude-haiku-4-5", 812, 96, (812*1 + 96*5) / 1e6}, // small-call arithmetic
	} {
		if got := c.Cost(tc.model, tc.in, tc.out); got != tc.want {
			t.Errorf("Cost(%q, %d, %d) = %v, want %v", tc.model, tc.in, tc.out, got, tc.want)
		}
	}
}

func TestCostWithPrices(t *testing.T) {
	c := a1.NewClient("test-key", a1.WithPrices(map[string]a1.Price{
		"sonnet":          {4, 20},
		"claude-sonnet-5": {1, 2},
	}))
	for _, tc := range []struct {
		model   string
		in, out int64
		want    float64
	}{
		{"claude-sonnet-4-6", 1_000_000, 0, 4}, // the override's family price
		{"claude-sonnet-5", 1_000_000, 0, 1},   // its per-model entry beats that
		{"claude-haiku-4-5", 1_000_000, 0, 1},  // unpriced here → the shipped table
		{"nothing-familiar", 1_000_000, 0, 10}, // unpriced anywhere → top tier
	} {
		if got := c.Cost(tc.model, tc.in, tc.out); got != tc.want {
			t.Errorf("Cost(%q, %d, %d) = %v, want %v", tc.model, tc.in, tc.out, got, tc.want)
		}
	}
}

// An override is consulted whole before the shipped table, so pricing a family
// covers a model the shipped table happens to name exactly.
func TestCostOverriddenFamilyBeatsShippedModel(t *testing.T) {
	c := a1.NewClient("test-key", a1.WithPrices(map[string]a1.Price{"sonnet": {4, 20}}))
	if got := c.Cost("claude-sonnet-5", 1_000_000, 0); got != 4 {
		t.Errorf("Cost = %v, want the override's family price 4", got)
	}
}

func TestWithPricesCopies(t *testing.T) {
	prices := map[string]a1.Price{"haiku": {2, 8}}
	c := a1.NewClient("test-key", a1.WithPrices(prices))
	prices["haiku"] = a1.Price{InUSD: 99, OutUSD: 99}
	if got := c.Cost("claude-haiku-4-5", 1_000_000, 0); got != 2 {
		t.Errorf("Cost = %v, want 2, not the caller's later mutation", got)
	}
}

func TestMetaCarriesCost(t *testing.T) {
	c, _ := newTestClientOpts(t,
		[]a1.Option{a1.WithPrices(map[string]a1.Price{"haiku": {2, 8}})},
		message("hi", "end_turn"))
	_, meta, err := c.Text(t.Context(), a1.Request{Task: "unit", Model: "m", Prompt: "p"})
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	// The scripted response reports model claude-haiku-4-5 with 100 in / 25 out.
	if want := (100*2.0 + 25*8.0) / 1e6; meta.CostUSD != want {
		t.Errorf("meta.CostUSD = %v, want %v", meta.CostUSD, want)
	}
}
