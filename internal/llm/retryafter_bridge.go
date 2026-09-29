package llm

import (
	"errors"
	"time"

	"github.com/vanpiyp/awp/internal/llm/protocol"
	"github.com/vanpiyp/awp/internal/llm/retryafter"
)

func RetryAfterFromAny(err error) time.Duration {
	if d := retryafter.RetryAfterFromError(err); d > 0 {
		return d
	}
	var httpErr *protocol.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.RetryAfter
	}
	return 0
}
