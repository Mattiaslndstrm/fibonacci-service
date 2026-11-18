package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Mattiaslndstrm/fibonacci-service/internal/fibonacci"
	"github.com/Mattiaslndstrm/fibonacci-service/internal/httpapi"
)

const (
	invalidRequest  = "/problems/invalid-request"
	needsString     = "/problems/n-too-large-for-number-format"
	contentType     = "Content-Type"
	cacheControl    = "Cache-Control"
	contentLength   = "Content-Length"
	pdfExampleQuery = "/fibonacci?n=5"
	pdfExampleBody  = "[0,1,1,2,3]"
	plainText       = "text/plain; charset=utf-8"
)

func TestEveryReplyMatchesEncodingJSON(t *testing.T) {
	h, _ := newHandler()

	for n := range 80 {
		want, err := json.Marshal(fibonacci.Sequence(n))
		require.NoError(t, err)

		rec := request(t, h, http.MethodGet, "/fibonacci?n="+strconv.Itoa(n))
		require.Equal(t, http.StatusOK, rec.Code, "n=%d", n)
		require.Equal(t, string(want), rec.Body.String(), "n=%d", n)
		require.Equal(t, strconv.Itoa(len(want)), rec.Result().Header.Get(contentLength), "n=%d", n)
	}

	for n := range 1001 {
		numbers := fibonacci.Sequence(n)
		texts := make([]string, len(numbers))

		for i, number := range numbers {
			texts[i] = number.String()
		}

		want, err := json.Marshal(texts)
		require.NoError(t, err)

		rec := request(t, h, http.MethodGet, "/fibonacci?format=string&n="+strconv.Itoa(n))
		require.Equal(t, http.StatusOK, rec.Code, "n=%d", n)
		require.Equal(t, string(want), rec.Body.String(), "n=%d", n)
		require.Equal(t, strconv.Itoa(len(want)), rec.Result().Header.Get(contentLength), "n=%d", n)
	}
}

func TestRepliesTypedOut(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "the PDF's example", target: pdfExampleQuery, want: pdfExampleBody},
		{name: "the example as strings", target: "/fibonacci?n=5&format=string", want: `["0","1","1","2","3"]`},
		{name: "no numbers", target: "/fibonacci?n=0", want: "[]"},
		{name: "the number format asked for by name", target: "/fibonacci?n=5&format=number", want: pdfExampleBody},
		{name: "leading zeros", target: "/fibonacci?n=007", want: "[0,1,1,2,3,5,8]"},
		{name: "a plus sign", target: "/fibonacci?n=%2B5", want: pdfExampleBody},
		{name: "minus zero", target: "/fibonacci?n=-0", want: "[]"},
	}

	h, _ := newHandler()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := request(t, h, http.MethodGet, tt.target)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.want, rec.Body.String())
		})
	}
}

func TestSuccessfulReplyHeaders(t *testing.T) {
	h, _ := newHandler()

	rec := request(t, h, http.MethodGet, pdfExampleQuery)

	assert.Equal(t, "application/json", rec.Result().Header.Get(contentType))
	assert.Equal(t, strconv.Itoa(len(pdfExampleBody)), rec.Result().Header.Get(contentLength))
	assert.Equal(t, "public, max-age=86400", rec.Result().Header.Get(cacheControl))
}

func TestInvalidRequestsGetTheirProblem(t *testing.T) {
	const (
		malformed   = "the query string is malformed"
		unknown     = "the only parameters are n and format"
		required    = "n is required"
		nOnce       = "n may be given only once"
		notInteger  = "n must be an integer"
		negative    = "n must not be negative"
		tooLarge    = "n must be at most 1000"
		formatOnce  = "format may be given only once"
		badFormat   = "format must be number or string"
		tooManyNums = "n above 79 needs format=string"
	)

	tests := []struct {
		name       string
		target     string
		wantType   string
		wantDetail string
	}{
		{"a query string that cannot be parsed", "/fibonacci?n=%zz", invalidRequest, malformed},
		{"a semicolon as separator", "/fibonacci?n=5;format=string", invalidRequest, malformed},
		{"an unknown parameter", "/fibonacci?n=5&fromat=string", invalidRequest, unknown},
		{"n in upper case", "/fibonacci?N=5", invalidRequest, unknown},
		{"no n", "/fibonacci", invalidRequest, required},
		{"an empty n", "/fibonacci?n=", invalidRequest, required},
		{"n twice", "/fibonacci?n=5&n=7", invalidRequest, nOnce},
		{"n in letters", "/fibonacci?n=abc", invalidRequest, notInteger},
		{"n with a decimal point", "/fibonacci?n=5.0", invalidRequest, notInteger},
		{"n with an exponent", "/fibonacci?n=1e3", invalidRequest, notInteger},
		{"n after a space", "/fibonacci?n=%205", invalidRequest, notInteger},
		{"a negative n", "/fibonacci?n=-1", invalidRequest, negative},
		{"a negative n beyond 64 bits", "/fibonacci?n=-99999999999999999999", invalidRequest, negative},
		{"one more than the maximum", "/fibonacci?n=1001&format=string", invalidRequest, tooLarge},
		{"n beyond 64 bits", "/fibonacci?n=99999999999999999999", invalidRequest, tooLarge},
		{"a letter after an n beyond 64 bits", "/fibonacci?n=99999999999999999999x", invalidRequest, notInteger},
		{"a letter after a negative n beyond 64 bits", "/fibonacci?n=-99999999999999999999x", invalidRequest, notInteger},
		{"format twice", "/fibonacci?n=5&format=string&format=number", invalidRequest, formatOnce},
		{"an unknown format", "/fibonacci?n=5&format=xml", invalidRequest, badFormat},
		{"an empty format", "/fibonacci?n=5&format=", invalidRequest, badFormat},
		{"format in upper case", "/fibonacci?n=5&format=String", invalidRequest, badFormat},
		{"one more than the number format holds", "/fibonacci?n=80", needsString, tooManyNums},
		{"an unknown parameter comes before a bad n", "/fibonacci?n=abc&fromat=string", invalidRequest, unknown},
		{"a bad n comes before a bad format", "/fibonacci?n=5000&format=xml", invalidRequest, tooLarge},
		{"a bad format comes before the number format's limit", "/fibonacci?n=100&format=xml", invalidRequest, badFormat},
	}

	h, _ := newHandler()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := `{"type":"` + tt.wantType + `","detail":"` + tt.wantDetail + `"}`

			rec := request(t, h, http.MethodGet, tt.target)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Result().Header.Get(contentType))
			assert.Equal(t, strconv.Itoa(len(want)), rec.Result().Header.Get(contentLength))
			assert.Empty(t, rec.Result().Header.Values(cacheControl))
			assert.Equal(t, want, rec.Body.String())
		})
	}
}

func TestBodyOnGetIsIgnored(t *testing.T) {
	h, _ := newHandler()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, pdfExampleQuery, strings.NewReader("n=7"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, pdfExampleBody, rec.Body.String())
}

func TestHeadGetsTheHeadersOfGet(t *testing.T) {
	h, _ := newHandler()

	get := request(t, h, http.MethodGet, pdfExampleQuery)
	head := request(t, h, http.MethodHead, pdfExampleQuery)

	assert.Equal(t, http.StatusOK, head.Code)
	assert.Equal(t, get.Result().Header, head.Result().Header)
}

func TestUnknownPathGets404(t *testing.T) {
	h, _ := newHandler()

	rec := request(t, h, http.MethodGet, "/nope")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, plainText, rec.Result().Header.Get(contentType))
}

func TestOtherMethodGets405WithAllow(t *testing.T) {
	h, _ := newHandler()

	rec := request(t, h, http.MethodPost, pdfExampleQuery)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, plainText, rec.Result().Header.Get(contentType))
	assert.Equal(t, "GET, HEAD", rec.Result().Header.Get("Allow"))
}

func TestHealthIsEmptyAndNotLogged(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			h, logs := newHandler()

			rec := request(t, h, method, "/healthz")

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Empty(t, rec.Body.String())
			assert.Equal(t, "0", rec.Result().Header.Get(contentLength))
			assert.Empty(t, rec.Result().Header.Values(cacheControl))
			assert.Empty(t, logs.String())
		})
	}
}

func TestSuccessfulRequestIsLogged(t *testing.T) {
	h, logs := newHandler()

	request(t, h, http.MethodGet, "/fibonacci?n=5&format=string")

	line := onlyLine(t, logs)
	assert.Equal(t, "INFO", line["level"])
	assert.Equal(t, "request", line["msg"])
	assert.Equal(t, http.MethodGet, line["method"])
	assert.Equal(t, "/fibonacci", line["path"])
	assert.Equal(t, "5", line["n"])
	assert.Equal(t, "string", line["format"])
	assert.InDelta(t, http.StatusOK, line["status"], 0)
	assert.IsType(t, float64(0), line["duration_ms"])
	assert.NotContains(t, line, "problem")
	assert.NotContains(t, line, "detail")
	assert.NotContains(t, line, "query")
	assert.NotContains(t, line, "error")
}

func TestFailedRequestIsLoggedWithItsProblem(t *testing.T) {
	h, logs := newHandler()

	request(t, h, http.MethodGet, "/fibonacci?n=-1")

	line := onlyLine(t, logs)
	assert.Equal(t, "-1", line["n"])
	assert.InDelta(t, http.StatusBadRequest, line["status"], 0)
	assert.Equal(t, invalidRequest, line["problem"])
	assert.Equal(t, "n must not be negative", line["detail"])
	assert.Equal(t, "n=-1", line["query"])
	assert.NotContains(t, line, "format")
}

func TestMalformedQueryIsLoggedAsSent(t *testing.T) {
	h, logs := newHandler()

	request(t, h, http.MethodGet, "/fibonacci?n=%zz")

	assert.Equal(t, "n=%zz", onlyLine(t, logs)["query"])
}

func TestReplyOfTheRouterIsLogged(t *testing.T) {
	h, logs := newHandler()

	request(t, h, http.MethodGet, "/nope")

	line := onlyLine(t, logs)
	assert.Equal(t, http.MethodGet, line["method"])
	assert.Equal(t, "/nope", line["path"])
	assert.InDelta(t, http.StatusNotFound, line["status"], 0)
}

func TestWriteErrorIsLogged(t *testing.T) {
	h, logs := newHandler()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, pdfExampleQuery, http.NoBody)

	h.ServeHTTP(brokenWriter{header: http.Header{}}, req)

	assert.Equal(t, errBrokenWriter.Error(), onlyLine(t, logs)["error"])
}

var errBrokenWriter = errors.New("the client is gone")

// brokenWriter is a ResponseWriter whose client has gone away.
type brokenWriter struct {
	header http.Header
}

func (w brokenWriter) Header() http.Header { return w.header }

func (brokenWriter) Write([]byte) (int, error) { return 0, errBrokenWriter }

func (brokenWriter) WriteHeader(int) {}

func newHandler() (*httpapi.Handler, *bytes.Buffer) {
	logs := &bytes.Buffer{}

	return httpapi.NewHandler(slog.New(slog.NewJSONHandler(logs, nil))), logs
}

func request(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, target, http.NoBody))

	return rec
}

func onlyLine(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()

	text := strings.TrimSuffix(logs.String(), "\n")
	require.NotContains(t, text, "\n", "want one log line")

	var line map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &line))

	return line
}
