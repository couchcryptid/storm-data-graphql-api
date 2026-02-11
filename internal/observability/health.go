package observability

import (
	"net/http"

	sharedobs "github.com/couchcryptid/storm-data-shared/observability"
)

// LivenessHandler returns 200 OK unconditionally.
func LivenessHandler() http.HandlerFunc {
	return sharedobs.LivenessHandler()
}

// ReadinessHandler checks downstream dependencies and returns 200 or 503.
func ReadinessHandler(checker ReadinessChecker) http.HandlerFunc {
	return sharedobs.ReadinessHandler(checker)
}
