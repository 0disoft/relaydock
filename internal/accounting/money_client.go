package accounting

import "context"

type MoneyClient interface {
	AuthorizeHold(context.Context, Quote) (Authorization, error)
	CaptureUsage(context.Context, Authorization, Usage) (Authorization, error)
	ReleaseHold(context.Context, Authorization, int64) error
	Adjust(context.Context, string, int64, string) error
}
