package httpclient

import "errors"

// Typed errors returned for each Python service error code and for
// client-level failures (timeout, hash mismatch, unexpected status).
var (
	ErrPromptVersionMismatch = errors.New("httpclient: prompt version mismatch")
	ErrPromptHashMismatch    = errors.New("httpclient: prompt hash mismatch")
	ErrProviderError         = errors.New("httpclient: provider error")
	ErrProviderTimeout       = errors.New("httpclient: provider timeout")
	ErrInvalidModelOutput    = errors.New("httpclient: invalid model output")
	ErrCoverageMismatch      = errors.New("httpclient: coverage mismatch")
	ErrUnknownCategory       = errors.New("httpclient: unknown category")
	ErrUnknownSubdomain      = errors.New("httpclient: unknown subdomain")
	ErrEmptyAnswer           = errors.New("httpclient: empty answer")
	ErrInvalidRequest        = errors.New("httpclient: invalid request")
	ErrHTTPError             = errors.New("httpclient: unexpected HTTP error")
)
