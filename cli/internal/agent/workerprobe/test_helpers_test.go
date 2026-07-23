package workerprobe_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type handlerRoundTripper struct {
	handler http.Handler
}

func (rt handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	body := []byte(nil)
	if req.Body != nil {
		defer func() { _ = req.Body.Close() }()
		body, _ = io.ReadAll(req.Body)
	}
	inProcessReq := httptest.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), bytes.NewReader(body))
	inProcessReq.Header = req.Header.Clone()
	inProcessReq.Host = req.Host
	inProcessReq.RemoteAddr = "127.0.0.1:54321"

	rec := httptest.NewRecorder()
	rt.handler.ServeHTTP(rec, inProcessReq)
	res := rec.Result()
	res.Request = req
	return res, nil
}

func newInProcessHTTPClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: handlerRoundTripper{handler: handler}}
}

func startHTTPServerOrSkip(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	var srv *httptest.Server
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		srv = httptest.NewServer(handler)
	}()
	if recovered != nil {
		t.Skipf("httptest.NewServer unavailable in this environment: %v", recovered)
	}
	t.Cleanup(srv.Close)
	return srv
}
