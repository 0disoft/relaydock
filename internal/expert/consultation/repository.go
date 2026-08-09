package consultation

import "context"

type Repository interface {
	Create(context.Context, CreateCommand) (Consultation, error)
	Get(context.Context, string) (Consultation, error)
	List(context.Context, int) ([]Consultation, error)
	UpdateState(context.Context, string, State, State) (Consultation, error)
	AttachContext(context.Context, string, string, State, State) (Consultation, error)
	ClaimNext(context.Context, string) (Consultation, error)
	AttachResult(context.Context, string, string) (Consultation, error)
	SetFailure(context.Context, string, string) (Consultation, error)
}
