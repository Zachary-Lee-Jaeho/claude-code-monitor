package data

type pricingTier struct {
	inputPerM       float64
	outputPerM      float64
	cacheCreatePerM float64
	cacheReadPerM   float64
}

var pricing = map[ModelTier]pricingTier{
	TierOpus: {
		inputPerM:       15.0,
		outputPerM:      75.0,
		cacheCreatePerM: 18.75,
		cacheReadPerM:   1.50,
	},
	TierSonnet: {
		inputPerM:       3.0,
		outputPerM:      15.0,
		cacheCreatePerM: 3.75,
		cacheReadPerM:   0.30,
	},
	TierHaiku: {
		inputPerM:       0.25,
		outputPerM:      1.25,
		cacheCreatePerM: 0.30,
		cacheReadPerM:   0.03,
	},
}

// CalculateCost returns (total_cost, saved_cost) for a given model and usage.
func CalculateCost(modelName string, u *TokenUsage) (float64, float64) {
	tier := ModelTierFrom(modelName)
	p := pricing[tier]
	m := 1_000_000.0

	totalCost := float64(u.InputTokens)/m*p.inputPerM +
		float64(u.OutputTokens)/m*p.outputPerM +
		float64(u.CacheCreationInputTokens)/m*p.cacheCreatePerM +
		float64(u.CacheReadInputTokens)/m*p.cacheReadPerM

	// Cost if cache reads were full-price input instead
	withoutCache := float64(u.InputTokens+u.CacheReadInputTokens)/m*p.inputPerM +
		float64(u.OutputTokens)/m*p.outputPerM

	saved := withoutCache - totalCost
	if saved < 0 {
		saved = 0
	}

	return totalCost, saved
}
