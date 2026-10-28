package server

import (
	"encoding/json"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"subtrack/internal/auth"
)

// TestSpecMatchesRouter pins docs/swagger.json against the mounted routes:
// every "<METHOD> <path>" under /api/v1 must be documented, and the spec
// must document nothing the router does not serve. Both drifts are silent
// otherwise: a spec missing an operation still generates and still loads.
// Regenerate with `make swagger`.
func TestSpecMatchesRouter(t *testing.T) {
	r := mux.NewRouter()
	routes(r, nil, auth.Config{}, time.UTC)

	mounted := map[string]bool{}
	err := r.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		methods, err := route.GetMethods()
		if err != nil {
			return nil // the per-path 405 fallback carries no method
		}
		for _, m := range methods {
			mounted[m+" "+normalize(tpl)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile("../../docs/swagger.json")
	if err != nil {
		t.Fatalf("read spec (run `make swagger`): %v", err)
	}
	var spec struct {
		BasePath string                    `json:"basePath"`
		Paths    map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(b, &spec); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			documented[strings.ToUpper(method)+" "+normalize(spec.BasePath+path)] = true
		}
	}

	var missing, phantom []string
	for op := range mounted {
		if strings.Contains(op, " /api/v1/") && !documented[op] {
			missing = append(missing, op)
		}
	}
	for op := range documented {
		if !mounted[op] {
			phantom = append(phantom, op)
		}
	}
	sort.Strings(missing)
	sort.Strings(phantom)
	if len(missing) > 0 {
		t.Errorf("routes missing from docs/swagger.json (run `make swagger`, check the @Router line):\n  %s", strings.Join(missing, "\n  "))
	}
	if len(phantom) > 0 {
		t.Errorf("docs/swagger.json documents routes nobody serves:\n  %s", strings.Join(phantom, "\n  "))
	}
	if len(mounted) < 40 || !slices.Contains(keys(mounted), "GET /api/v1/analytics/export") {
		t.Errorf("router walk found only %d routes", len(mounted))
	}
}

// normalize drops gorilla's {id:[0-9]+} patterns so both sides read {id}.
func normalize(path string) string {
	var b strings.Builder
	depth := 0
	skip := false
	for _, c := range path {
		switch {
		case c == '{':
			depth++
			skip = false
		case c == '}':
			depth--
			skip = false
		case c == ':' && depth == 1:
			skip = true
			continue
		}
		if !skip {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
