// Package httpapi serves the Fibonacci sequence over HTTP.
package httpapi

import (
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Mattiaslndstrm/fibonacci-service/internal/fibonacci"
)

const (
	maxN = 1000
	// F(78) is the largest Fibonacci number a float64 holds exactly (<2^53).
	maxNumberN = 79

	paramN      = "n"
	paramFormat = "format"

	formatNumber = "number"
	formatString = "string"

	typeInvalidRequest = "/problems/invalid-request"
	typeNeedsString    = "/problems/n-too-large-for-number-format"
)

// Handler answers the service's HTTP requests and writes the request log.
type Handler struct {
	logger    *slog.Logger
	mux       *http.ServeMux
	asNumbers sequenceText
	asStrings sequenceText
}

// NewHandler computes the Fibonacci numbers once and returns a handler.
func NewHandler(logger *slog.Logger) *Handler {
	numbers := fibonacci.Sequence(maxN)
	h := &Handler{
		logger:    logger,
		mux:       http.NewServeMux(),
		asNumbers: newSequenceText(numbers[:maxNumberN], false),
		asStrings: newSequenceText(numbers, true),
	}

	h.handle("GET /fibonacci", h.fibonacci)
	h.handle("GET /healthz", health)

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &recorder{ResponseWriter: w, status: http.StatusOK}

	h.mux.ServeHTTP(rec, r)

	if rec.quiet {
		return
	}

	h.logger.LogAttrs(r.Context(), slog.LevelInfo, "request", rec.attrs(r, time.Since(start))...)
}

// Each route receives the *recorder that ServeHTTP passes to the mux.
func (h *Handler) handle(pattern string, route func(*recorder, *http.Request)) {
	h.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		rec, ok := w.(*recorder)
		if !ok {
			panic("httpapi: route called without a recorder")
		}

		route(rec, r)
	})
}

func (h *Handler) fibonacci(rec *recorder, r *http.Request) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		rec.fail(invalid("the query string is malformed"))

		return
	}

	rec.query = query

	body, p := h.body(query)
	if p != nil {
		rec.fail(p)

		return
	}

	header := rec.Header()
	header.Set("Content-Type", "application/json")
	header.Set("Content-Length", strconv.Itoa(len(body)+1))
	header.Set("Cache-Control", "public, max-age=86400")
	rec.send(body, []byte{']'})
}

// body returns the reply for a query, or the first problem it finds. The reply lacks its closing bracket.
func (h *Handler) body(query url.Values) ([]byte, *problem) {
	for name := range query {
		if name != paramN && name != paramFormat {
			return nil, invalid("the only parameters are n and format")
		}
	}

	n, p := parseN(query[paramN])
	if p != nil {
		return nil, p
	}

	format, p := parseFormat(query[paramFormat])
	if p != nil {
		return nil, p
	}

	if format == formatString {
		return h.asStrings.first(n), nil
	}

	if n > maxNumberN {
		return nil, &problem{typ: typeNeedsString, detail: "n above " + strconv.Itoa(maxNumberN) + " needs format=string"}
	}

	return h.asNumbers.first(n), nil
}

func health(rec *recorder, _ *http.Request) {
	rec.quiet = true
	rec.Header().Set("Content-Length", "0")
}

func parseN(values []string) (int, *problem) {
	switch {
	case len(values) == 0, len(values) == 1 && values[0] == "":
		return 0, invalid("n is required")
	case len(values) > 1:
		return 0, invalid("n may be given only once")
	}

	n, ok := new(big.Int).SetString(values[0], 10)
	if !ok {
		return 0, invalid("n must be an integer")
	}

	if n.Sign() < 0 {
		return 0, invalid("n must not be negative")
	}

	if n.Cmp(big.NewInt(maxN)) > 0 {
		return 0, invalid("n must be at most " + strconv.Itoa(maxN))
	}

	return int(n.Int64()), nil
}

func parseFormat(values []string) (string, *problem) {
	switch {
	case len(values) == 0:
		return formatNumber, nil
	case len(values) > 1:
		return "", invalid("format may be given only once")
	}

	switch values[0] {
	case formatNumber, formatString:
		return values[0], nil
	default:
		return "", invalid("format must be " + formatNumber + " or " + formatString)
	}
}

// sequenceText holds the JSON text of all the numbers. The reply for n is text[:ends[n]] followed by "]".
type sequenceText struct {
	text []byte
	ends []int
}

func newSequenceText(numbers []*big.Int, quoted bool) sequenceText {
	st := sequenceText{text: []byte{'['}, ends: make([]int, len(numbers)+1)}
	st.ends[0] = len(st.text)

	for i, number := range numbers {
		if i > 0 {
			st.text = append(st.text, ',')
		}

		if quoted {
			st.text = append(st.text, '"')
		}

		st.text = number.Append(st.text, 10)

		if quoted {
			st.text = append(st.text, '"')
		}

		st.ends[i+1] = len(st.text)
	}

	return st
}

func (st sequenceText) first(n int) []byte {
	// The third index sets the capacity of the slice, so that append cannot overwrite text.
	return st.text[:st.ends[n]:st.ends[n]]
}

// problem is an RFC 9457 problem. The service answers each with status 400.
type problem struct {
	typ    string
	detail string
}

func invalid(detail string) *problem {
	return &problem{typ: typeInvalidRequest, detail: detail}
}

// recorder wraps the ResponseWriter of one request.
// It remembers the query, the status, the problem and the write error for the log line.
type recorder struct {
	http.ResponseWriter

	query   url.Values
	problem *problem
	err     error
	status  int
	quiet   bool
}

func (rec *recorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *recorder) fail(p *problem) {
	rec.problem = p

	// %q writes a valid JSON string here because every type and detail is printable ASCII.
	body := fmt.Appendf(nil, `{"type":%q,"detail":%q}`, p.typ, p.detail)

	header := rec.Header()
	header.Set("Content-Type", "application/problem+json")
	header.Set("Content-Length", strconv.Itoa(len(body)))
	rec.WriteHeader(http.StatusBadRequest)
	rec.send(body)
}

func (rec *recorder) send(parts ...[]byte) {
	for _, part := range parts {
		//nolint:gosec // G705 does not apply: no reply contains text from the request.
		if _, err := rec.Write(part); err != nil {
			rec.err = err

			return
		}
	}
}

func (rec *recorder) attrs(r *http.Request, duration time.Duration) []slog.Attr {
	attrs := []slog.Attr{slog.String("method", r.Method), slog.String("path", r.URL.Path)}

	for _, name := range []string{paramN, paramFormat} {
		if values, ok := rec.query[name]; ok {
			attrs = append(attrs, slog.String(name, values[0]))
		}
	}

	attrs = append(attrs,
		slog.Int("status", rec.status),
		slog.Float64("duration_ms", float64(duration)/float64(time.Millisecond)),
	)

	if rec.problem != nil {
		attrs = append(attrs,
			slog.String("problem", rec.problem.typ),
			slog.String("detail", rec.problem.detail),
			slog.String("query", r.URL.RawQuery),
		)
	}

	if rec.err != nil {
		attrs = append(attrs, slog.String("error", rec.err.Error()))
	}

	return attrs
}
