package workerprobe

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
)

type inProcessHTTPServer struct {
	URL    string
	Client *http.Client
}

func (s *inProcessHTTPServer) Close() {}

func newInProcessHTTPServer(handler http.Handler) *inProcessHTTPServer {
	return &inProcessHTTPServer{
		URL:    "http://workerprobe.test",
		Client: newInProcessHTTPClient(handler),
	}
}

func newInProcessHTTPClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: handlerRoundTripper{handler: handler}}
}

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
