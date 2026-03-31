package builders

import "errors"

var (
	ErrBuilderNotFound      = errors.New("builder not found")
	ErrBuilderNotAvailable  = errors.New("builder not available")
	ErrBuilderAlreadyLeased = errors.New("builder already leased")
	ErrLeaseNotFound        = errors.New("lease not found")
	ErrLeaseExpired         = errors.New("lease expired")
	ErrNoBuilderCandidate   = errors.New("no builder candidate")
	ErrInvalidRequest       = errors.New("invalid request")
)
