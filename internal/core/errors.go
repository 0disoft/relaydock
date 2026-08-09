package core

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
