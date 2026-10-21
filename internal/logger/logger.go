// Package logger builds the application's *slog.Logger.
//
// Two output formats:
//
//   - "json"    - slog.JSONHandler. Default when stdout is not a TTY (piped to
//     a file, container log driver, log aggregator, etc.).
//   - "console" - a small custom handler that emits one human-readable line per
//     record with the LEVEL token ANSI-colored per level. Default when stdout
//     is a terminal.
//
// Output goes to **stdout** (12-factor convention; also avoids JetBrains IDEs
// painting every stderr line red regardless of content).
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type ctxKey struct{}

// ANSI SGR codes - kept inline so the package has no external dependencies.
const (
	ansiReset  = "\x1b[0m"
	ansiGray   = "\x1b[90m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiBold   = "\x1b[1m"
)

// New constructs the application logger from a Config.
func New(cfg Config) *slog.Logger { return NewTo(os.Stdout, cfg) }

// NewTo is the same as New but writes to an arbitrary io.Writer (useful for
// tests). When cfg.Format is "" or "auto", JSON is chosen unless w is a
// terminal, in which case console is chosen.
func NewTo(w io.Writer, cfg Config) *slog.Logger {
	lvl := parseLevel(cfg.Level)
	switch resolveFormat(cfg.Format, w) {
	case "console":
		return slog.New(&consoleHandler{w: w, level: lvl, mu: &sync.Mutex{}})
	default:
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level:       lvl,
			AddSource:   true,
			ReplaceAttr: jsonReplaceAttr,
		}))
	}
}

// jsonReplaceAttr collapses slog's default `source` object - `{function,file,line}` -
// into a single "file:line" string. The function name (e.g.
// `main.run.run.Logging.func4.func5`) is auto-generated for nested closures
// and rarely informative; file:line is what an operator actually navigates to.
// File paths are trimmed to the module-relative form so the JSON line stays
// short.
func jsonReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.SourceKey {
		if src, ok := a.Value.Any().(*slog.Source); ok && src != nil {
			return slog.String(slog.SourceKey, fmt.Sprintf("%s:%d", trimSourcePath(src.File), src.Line))
		}
	}
	return a
}

// trimSourcePath cuts everything before the module's top-level code dirs
// (internal/, cmd/). It anchors on those rather than on the checkout directory
// name, which differs between machines.
func trimSourcePath(p string) string {
	for _, anchor := range []string{"/internal/", "/cmd/"} {
		if i := strings.LastIndex(p, anchor); i >= 0 {
			return p[i+1:]
		}
	}
	return p
}

func parseLevel(s string) slog.Level {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.ToLower(s))); err != nil {
		return slog.LevelInfo
	}
	return lvl
}

func resolveFormat(format string, w io.Writer) string {
	switch strings.ToLower(format) {
	case "json", "console":
		return strings.ToLower(format)
	}
	if isTerminal(w) {
		return "console"
	}
	return "json"
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// consoleHandler renders records as
//
//	HH:MM:SS.mmm <LEVEL> message  key=value key=value
//
// where <LEVEL> carries an ANSI color (gray/green/yellow/red for
// debug/info/warn/error) and key= attribute names are dim. Non-trivial groups
// and complex Value types fall back to fmt.Sprint - adequate for an internal
// service log; production aggregation should use the JSON format.
type consoleHandler struct {
	w     io.Writer
	level slog.Level
	mu    *sync.Mutex // shared across handler clones from WithAttrs / WithGroup
	attrs []slog.Attr
	group string // empty = no group prefix
}

// Enabled reports whether the handler will emit records at level l.
func (h *consoleHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

// Handle renders one record to the handler's writer. The shared mutex is
// held during the write so concurrent loggers do not interleave bytes.
func (h *consoleHandler) Handle(_ context.Context, r slog.Record) error {
	b := make([]byte, 0, 256)

	b = append(b, ansiGray...)
	b = r.Time.AppendFormat(b, "15:04:05.000")
	b = append(b, ansiReset...)
	b = append(b, ' ')

	b = append(b, levelColor(r.Level)...)
	b = append(b, levelLabel(r.Level)...)
	b = append(b, ansiReset...)
	b = append(b, ' ')

	b = append(b, r.Message...)

	for _, a := range h.attrs {
		b = appendAttr(b, h.group, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		b = appendAttr(b, h.group, a)
		return true
	})
	b = append(b, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(b)
	return err
}

// WithAttrs returns a clone of the handler that prepends the given attrs
// to every emitted record. The clone shares the underlying mutex so
// writes from clones still serialize against each other.
func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	nh.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &nh
}

// WithGroup returns a clone of the handler that prefixes attribute keys
// with name (dot-joined when nested). An empty name is a no-op, matching
// slog.Handler semantics.
func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	if h.group == "" {
		nh.group = name
	} else {
		nh.group = h.group + "." + name
	}
	return &nh
}

// appendAttr renders one slog attribute into b in `key=value` form, with
// the key painted dim and prefixed by the optional group path. Empty
// attributes are skipped.
func appendAttr(b []byte, group string, a slog.Attr) []byte {
	if a.Equal(slog.Attr{}) {
		return b
	}
	b = append(b, ' ')
	b = append(b, ansiGray...)
	if group != "" {
		b = append(b, group...)
		b = append(b, '.')
	}
	b = append(b, a.Key...)
	b = append(b, '=')
	b = append(b, ansiReset...)
	b = fmt.Appendf(b, "%v", a.Value.Any())
	return b
}

// levelColor returns the ANSI color prefix for a log level - red for
// error, yellow for warn, green for info, gray for debug.
func levelColor(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return ansiBold + ansiRed
	case l >= slog.LevelWarn:
		return ansiYellow
	case l >= slog.LevelInfo:
		return ansiGreen
	default:
		return ansiGray
	}
}

// levelLabel returns the level name padded to 5 chars so columns line up.
func levelLabel(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARN "
	case l >= slog.LevelInfo:
		return "INFO "
	default:
		return "DEBUG"
	}
}

// WithContext returns a new context carrying the given logger. Use this to
// attach per-request fields (request id, route, user) once and have downstream
// code retrieve the enriched logger via FromContext.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext retrieves the logger attached to ctx, or slog.Default() if none.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// Op returns the request-scoped logger from ctx with an "op" attribute
// pre-bound. Callers use it at the top of a service or repository method to
// tag every subsequent log line with the layer + operation that produced it,
// keeping request_id and other transport-bound fields intact.
func Op(ctx context.Context, op string) *slog.Logger {
	return FromContext(ctx).With("op", op)
}

// DebugEnabled reports whether the context's logger would emit a DEBUG record.
// Guard a Debug call with it only when assembling the attributes is itself
// expensive - counting a result set, marshalling a payload, joining a slice of
// barcodes. slog already discards below-level records cheaply, so a Debug call
// whose arguments are plain fields needs no guard.
func DebugEnabled(ctx context.Context) bool {
	return FromContext(ctx).Enabled(ctx, slog.LevelDebug)
}
