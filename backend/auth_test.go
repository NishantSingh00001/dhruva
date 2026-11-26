package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func req(a *App, method, path, remote string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = remote
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, r)
	return rec
}

func TestAPIKeyIsRequiredWhenConfigured(t *testing.T) {
	a := newApp("")
	a.apiKey = "s3cret"
	cases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		want    int
	}{
		{"no key", "GET", "/vehicles", nil, 401},
		{"wrong key", "GET", "/vehicles", map[string]string{"X-API-Key": "nope"}, 401},
		{"header key", "GET", "/vehicles", map[string]string{"X-API-Key": "s3cret"}, 200},
		{"bearer key", "GET", "/vehicles", map[string]string{"Authorization": "Bearer s3cret"}, 200},
		{"query key rejected on normal routes", "GET", "/vehicles?key=s3cret", nil, 401},
		{"health stays open", "GET", "/health", nil, 200},
		{"preflight stays open", "OPTIONS", "/vehicles", nil, 204},
		{"write without key", "POST", "/vehicles", nil, 401},
	}
	for _, c := range cases {
		if got := req(a, c.method, c.path, "1.1.1.1:1", c.headers).Code; got != c.want {
			t.Errorf("%s: want %d got %d", c.name, c.want, got)
		}
	}
}

func TestStreamAcceptsQueryKeyBecauseEventSourceCannotSetHeaders(t *testing.T) {
	a := newApp("")
	a.apiKey = "s3cret"
	if got := req(a, "GET", "/ws/alerts", "1.1.1.1:1", nil).Code; got != 401 {
		t.Fatalf("stream without key: %d", got)
	}
	srv := httptest.NewServer(a.routes())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/ws/alerts?key=s3cret")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("stream with query key: %v %v", err, resp)
	}
	resp.Body.Close()
}

func TestRateLimitPerClientWithRefill(t *testing.T) {
	a := newApp("")
	now := time.Unix(1000, 0)
	a.limiter = newLimiter(2, 3, func() time.Time { return now }) // 2 req/s, burst 3
	for i := 0; i < 3; i++ {
		if got := req(a, "GET", "/vehicles", "9.9.9.9:1", nil).Code; got != 200 {
			t.Fatalf("burst request %d: %d", i, got)
		}
	}
	blocked := req(a, "GET", "/vehicles", "9.9.9.9:2", nil) // same IP, different port
	if blocked.Code != 429 || blocked.Header().Get("Retry-After") == "" {
		t.Fatalf("want 429 + Retry-After, got %d %v", blocked.Code, blocked.Header())
	}
	if got := req(a, "GET", "/vehicles", "8.8.8.8:1", nil).Code; got != 200 {
		t.Fatalf("other client must not be limited: %d", got)
	}
	if got := req(a, "GET", "/health", "9.9.9.9:1", nil).Code; got != 200 {
		t.Fatalf("health must bypass the limiter: %d", got)
	}
	now = now.Add(time.Second) // refills 2 tokens
	if got := req(a, "GET", "/vehicles", "9.9.9.9:1", nil).Code; got != 200 {
		t.Fatalf("after refill: %d", got)
	}
}

func TestLimiterForgetsIdleClients(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLimiter(1, 1, func() time.Time { return now })
	for i := 0; i < 100; i++ {
		l.allow(string(rune('a' + i)))
	}
	now = now.Add(time.Hour)
	l.allow("fresh")
	if n := l.size(); n > 2 {
		t.Fatalf("idle buckets not evicted: %d", n)
	}
}

func TestForwardedForOnlyTrustedBehindProxy(t *testing.T) {
	a := newApp("")
	a.limiter = newLimiter(1, 1, time.Now)
	h := map[string]string{"X-Forwarded-For": "5.5.5.5"}
	req(a, "GET", "/vehicles", "10.0.0.1:1", h)
	if got := req(a, "GET", "/vehicles", "10.0.0.1:1", map[string]string{"X-Forwarded-For": "6.6.6.6"}).Code; got != 429 {
		t.Fatalf("spoofed XFF must not bypass the limit when proxy is untrusted: %d", got)
	}
	a2 := newApp("")
	a2.trustProxy = true
	a2.limiter = newLimiter(1, 1, time.Now)
	req(a2, "GET", "/vehicles", "10.0.0.1:1", h)
	if got := req(a2, "GET", "/vehicles", "10.0.0.1:1", map[string]string{"X-Forwarded-For": "6.6.6.6"}).Code; got != 200 {
		t.Fatalf("trusted proxy should key on forwarded address: %d", got)
	}
}
