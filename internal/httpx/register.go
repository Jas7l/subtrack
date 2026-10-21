package httpx

import (
	"net/http"

	"github.com/gorilla/mux"
)

// Register attaches one handler per HTTP method to a path, plus a method-less
// fallthrough that emits 405 for any other verb (PUT, OPTIONS, etc.). Routes
// are matched in registration order, so the method-specific routes win for
// known verbs and the trailing catch-all only fires for unknown ones.
func Register(r *mux.Router, path string, handlers map[string]http.HandlerFunc) {
	for method, h := range handlers {
		r.HandleFunc(path, h).Methods(method)
	}
	r.HandleFunc(path, MethodNotAllowed)
}
