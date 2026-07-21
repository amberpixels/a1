package a1

import "strings"

// Per-MTok USD list prices by model family, matched by substring in the model
// id. Kept current by hand — prices move rarely, and a1 releases are cheap.
// Unknown models are billed at the most expensive known tier: a budget guard
// must overestimate, never underestimate.
var familyPrices = []struct {
	family string
	inUSD  float64 // per million input tokens
	outUSD float64 // per million output tokens
}{
	{"haiku", 1, 5},
	{"sonnet", 3, 15},
	{"opus", 5, 25},
	{"fable", 10, 50},
	{"mythos", 10, 50},
}

// fallback for model ids matching no known family — the top-tier price.
const (
	fallbackInUSD  = 10
	fallbackOutUSD = 50
)

// Cost estimates the USD list-price cost of one call. It is an estimate for
// budgeting/metering (no batch or cache discounts), not a billing statement.
func Cost(model string, inputTokens, outputTokens int64) float64 {
	inUSD, outUSD := float64(fallbackInUSD), float64(fallbackOutUSD)
	for _, p := range familyPrices {
		if strings.Contains(model, p.family) {
			inUSD, outUSD = p.inUSD, p.outUSD
			break
		}
	}
	return (float64(inputTokens)*inUSD + float64(outputTokens)*outUSD) / 1e6
}
