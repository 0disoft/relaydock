package accounting

import "time"

type Quote struct {
	ID, PriceRevisionID, Model string
	MaximumChargeMinor         int64
	ExpiresAt                  time.Time
}
type Authorization struct {
	ID, QuoteID   string
	HeldMinor     int64
	CapturedMinor int64
	Status        string
	ExpiresAt     time.Time
}
type Usage struct {
	RequestID, AttemptID                                                                             string
	InputTokens, CacheReadTokens, CacheWriteTokens, OutputTokens, ReasoningTokens, ProviderCostMinor int64
}
type Price struct {
	Model, RevisionID                                                                                                          string
	InputPerMillionMinor, CacheReadPerMillionMinor, CacheWritePerMillionMinor, OutputPerMillionMinor, ReasoningPerMillionMinor int64
}
