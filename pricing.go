package a1

import "strings"

// Price is the per-MTok USD list price of a model.
type Price struct {
	InUSD  float64 // per million input tokens
	OutUSD float64 // per million output tokens
}

// Per-MTok USD list prices, keyed by any substring of a model id: a family
// ("sonnet"), or a full id ("claude-sonnet-5") where one model diverges from
// its family. Kept current by hand, and correctable per client with WithPrices
// so a stale or negotiated row never waits for an a1 release.
var defaultPrices = map[string]Price{
	"haiku":           {1, 5},
	"sonnet":          {3, 15}, // Sonnet 4.6 and the rest of the family
	"claude-sonnet-5": {2, 10}, // diverges from it
	"opus":            {5, 25},
	"fable":           {10, 50},
	"mythos":          {10, 50},
}

// fallbackPrice bills a model id matching no known family at the most
// expensive known tier: a budget guard must overestimate, never underestimate.
var fallbackPrice = Price{10, 50}

// Cost estimates the USD list-price cost of one call at this client's prices.
// It is an estimate for budgeting/metering (no batch or cache discounts), not
// a billing statement.
func (c *Client) Cost(model string, inputTokens, outputTokens int64) float64 {
	p, ok := priceFor(c.prices, model)
	if !ok {
		if p, ok = priceFor(defaultPrices, model); !ok {
			p = fallbackPrice
		}
	}
	return (float64(inputTokens)*p.InUSD + float64(outputTokens)*p.OutUSD) / 1e6
}

// priceFor picks the entry whose key is the longest substring of model, so a
// per-model entry beats its family. Equal-length keys break on the smaller
// one, since map iteration order is not stable.
func priceFor(prices map[string]Price, model string) (Price, bool) {
	var (
		best  string
		price Price
		found bool
	)
	for key, p := range prices {
		if key == "" || !strings.Contains(model, key) {
			continue
		}
		if !found || len(key) > len(best) || (len(key) == len(best) && key < best) {
			best, price, found = key, p, true
		}
	}
	return price, found
}
