package cmd

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

type staticHTTPTransport struct {
	t              *testing.T
	payload        []byte
	contentType    string
	wantPathSuffix string
}

func (rt *staticHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.t.Helper()

	if req.Method != http.MethodGet {
		rt.t.Fatalf("unexpected method: got %s want GET", req.Method)
	}
	if rt.wantPathSuffix != "" && !strings.HasSuffix(req.URL.Path, rt.wantPathSuffix) {
		rt.t.Fatalf("unexpected request path: got %s want suffix %s", req.URL.Path, rt.wantPathSuffix)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(rt.payload)),
		Header:     make(http.Header),
		Request:    req,
	}
	if rt.contentType != "" {
		resp.Header.Set("Content-Type", rt.contentType)
	}
	return resp, nil
}

func withStaticHTTPTransport(t *testing.T, payload []byte, contentType, wantPathSuffix string) {
	t.Helper()

	orig := http.DefaultTransport
	http.DefaultTransport = &staticHTTPTransport{
		t:              t,
		payload:        payload,
		contentType:    contentType,
		wantPathSuffix: wantPathSuffix,
	}
	t.Cleanup(func() {
		http.DefaultTransport = orig
	})
}
