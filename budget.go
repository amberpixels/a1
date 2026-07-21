package a1

// Budget declares a spend policy in code: a default plus the operator-tunable
// range. Configuration can customize the value within [Min, Max]; anything
// else — unset, zero, negative, out of range — resolves to Default. Unlimited
// is deliberately not expressible: a missing or bogus config value must never
// disable a spend guard.
//
// The unit is whatever the declaring app counts in (USD/day is the common
// case); Budget only owns the clamping semantics, not the counting.
type Budget struct {
	Default float64
	Min     float64
	Max     float64
}

// Resolve maps an operator-configured value to the effective one: the value
// itself when it lies within [Min, Max], Default otherwise.
func (b Budget) Resolve(configured float64) float64 {
	if configured < b.Min || configured > b.Max {
		return b.Default
	}
	return configured
}
