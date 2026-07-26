package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbe(t *testing.T) {
	m := &Monitor{client: &http.Client{Timeout: 2 * time.Second}}

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer up.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer down.Close()

	if s := m.probe(context.Background(), Endpoint{Name: "up", URL: up.URL}); !s.Up || s.Code != 200 {
		t.Fatalf("expected up/200, got up=%v code=%d", s.Up, s.Code)
	}
	if s := m.probe(context.Background(), Endpoint{Name: "down", URL: down.URL}); s.Up || s.Code != 500 {
		t.Fatalf("expected down/500, got up=%v code=%d", s.Up, s.Code)
	}
	// unreachable host -> down with an error, no panic
	if s := m.probe(context.Background(), Endpoint{Name: "bad", URL: "http://127.0.0.1:1"}); s.Up || s.Error == "" {
		t.Fatalf("expected down with error, got up=%v err=%q", s.Up, s.Error)
	}
}

func TestProbeAssertions(t *testing.T) {
	m := &Monitor{client: &http.Client{Timeout: 2 * time.Second}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	}))
	defer srv.Close()

	// body assertion satisfied
	if s := m.probe(context.Background(), Endpoint{URL: srv.URL, ExpectBody: "pong"}); !s.Up {
		t.Fatalf("expected up when body matches, got down: %q", s.Error)
	}
	// body assertion failed -> down even though status is 200
	if s := m.probe(context.Background(), Endpoint{URL: srv.URL, ExpectBody: "nope"}); s.Up {
		t.Fatalf("expected down when body missing expected content")
	}
	// exact status assertion mismatch -> down even though 200 < 400
	if s := m.probe(context.Background(), Endpoint{URL: srv.URL, ExpectStatus: 204}); s.Up {
		t.Fatalf("expected down when status != expect_status")
	}
}
