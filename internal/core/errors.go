package core

/* llmnav/1 module
id=relaydock.core.error-contract
role=Define stable cross-layer error identities used for transport status mapping, retry decisions, and operator diagnostics.
owns=domain error identities|cross-layer failure vocabulary
excludes=provider error classification|user-facing message redaction
search=RelayDock domain errors|error identity mapping|shared failure codes
invariant=Callers classify failures with errors.Is rather than matching message text.
invariant=New identities remain provider-neutral and preserve existing transport mappings.
stability=contract
*/

import "errors"

var (
	ErrInvalidArgument       = errors.New("invalid argument")
	ErrInvalidConfiguration  = errors.New("invalid configuration")
	ErrInvalidTransition     = errors.New("invalid state transition")
	ErrCapabilityMismatch    = errors.New("requested capability is not supported")
	ErrLossyTransformation   = errors.New("protocol transformation would lose meaning")
	ErrBudgetExceeded        = errors.New("budget exceeded")
	ErrRateLimited           = errors.New("rate limited")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrForbidden             = errors.New("forbidden")
	ErrNotFound              = errors.New("not found")
	ErrConflict              = errors.New("conflict")
	ErrNoRoute               = errors.New("no eligible route")
	ErrProviderNotConfigured = errors.New("provider is not configured")
	ErrFrameTooLarge         = errors.New("frame exceeds configured limit")
	ErrPlatformUnsupported   = errors.New("platform is not supported")
	ErrClosed                = errors.New("resource is closed")
	ErrLeaseLost             = errors.New("worker lease lost")
	ErrExpired               = errors.New("resource expired")
	ErrNotModified           = errors.New("not modified")
	ErrCorruptState          = errors.New("corrupt state")
)
