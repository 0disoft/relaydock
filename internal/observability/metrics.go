package observability

import "context"

type Metrics interface {
	ObserveRequest(context.Context, RequestObservation)
	ObserveProviderAttempt(context.Context, ProviderAttemptObservation)
}

type RequestObservation struct {
	Protocol string
	Model    string
	Outcome  string
}

type ProviderAttemptObservation struct {
	Provider string
	Model    string
	Outcome  string
}
