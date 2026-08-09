package policy

type RoutingPolicy struct {
	ID                 string
	AllowedProviders   []string
	AllowedRegions     []string
	MaximumRequestCost int64
	LossMode           string
	RetryBeforeStream  int
	PayloadRetention   string
}
