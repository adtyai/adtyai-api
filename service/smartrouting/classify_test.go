package smartrouting

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
)

func TestClassifyStatusCodes(t *testing.T) {
	mk := func(code int, msg string) *types.NewAPIError {
		return types.NewErrorWithStatusCode(errors.New(msg), types.ErrorCodeBadResponseStatusCode, code)
	}
	if Classify(mk(400, "bad")) != ClassUserError {
		t.Fatal("400")
	}
	if Classify(mk(401, "unauthorized")) != ClassBanned {
		t.Fatal("401")
	}
	if Classify(mk(429, "rate")) != ClassRateLimit {
		t.Fatal("429")
	}
	if Classify(mk(500, "boom")) != ClassServer {
		t.Fatal("500")
	}
	if Classify(mk(402, "insufficient_quota")) != ClassBalance {
		t.Fatal("402")
	}
}
