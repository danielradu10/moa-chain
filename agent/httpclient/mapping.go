package httpclient

import "fmt"

// mapPythonError converts a Python service error code into the corresponding
// typed Go error. Unknown codes fall through to ErrHTTPError.
func mapPythonError(code, message string) error {
	switch code {
	case "PROMPT_VERSION_MISMATCH":
		return fmt.Errorf("%w: %s", ErrPromptVersionMismatch, message)
	case "PROVIDER_ERROR":
		return fmt.Errorf("%w: %s", ErrProviderError, message)
	case "PROVIDER_TIMEOUT":
		return fmt.Errorf("%w: %s", ErrProviderTimeout, message)
	case "INVALID_MODEL_OUTPUT":
		return fmt.Errorf("%w: %s", ErrInvalidModelOutput, message)
	case "COVERAGE_MISMATCH":
		return fmt.Errorf("%w: %s", ErrCoverageMismatch, message)
	case "UNKNOWN_CATEGORY":
		return fmt.Errorf("%w: %s", ErrUnknownCategory, message)
	case "UNKNOWN_SUBDOMAIN":
		return fmt.Errorf("%w: %s", ErrUnknownSubdomain, message)
	case "EMPTY_ANSWER":
		return fmt.Errorf("%w: %s", ErrEmptyAnswer, message)
	case "INVALID_REQUEST":
		return fmt.Errorf("%w: %s", ErrInvalidRequest, message)
	default:
		return fmt.Errorf("%w: code=%s: %s", ErrHTTPError, code, message)
	}
}
