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
