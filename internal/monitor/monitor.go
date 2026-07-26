package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
)

// LoadEndpoints reads the target list from a JSON file.
func LoadEndpoints(path string) ([]Endpoint, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var eps []Endpoint
	return eps, json.Unmarshal(b, &eps)
}

const failThreshold = 3 // page after this many consecutive failed checks

type Monitor struct {
	endpoints []Endpoint
	client    *http.Client
	store     *Store
	hub       *Hub

	mu     sync.Mutex
	fails  map[string]int    // consecutive failures per endpoint
	latest map[string]Status // current snapshot
}

func NewMonitor(eps []Endpoint, store *Store, hub *Hub) *Monitor {
	return &Monitor{
		endpoints: eps,
		client:    &http.Client{Timeout: 10 * time.Second},
		store:     store,
		hub:       hub,
		fails:     map[string]int{},
		latest:    map[string]Status{},
	}
}

// probe does one HTTP GET and reports up/down + latency, honouring the
// endpoint's optional status/body assertions.
func (m *Monitor) probe(ctx context.Context, e Endpoint) Status {
	st := Status{Name: e.Name, URL: e.URL, Ts: time.Now().UTC()}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, nil)
	start := time.Now()
	resp, err := m.client.Do(req)
	st.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	defer resp.Body.Close()
	st.Code = resp.StatusCode

	if e.ExpectStatus != 0 {
		st.Up = resp.StatusCode == e.ExpectStatus
	} else {
		st.Up = resp.StatusCode < 400
	}
	if !st.Up {
		st.Error = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		return st
	}

	if e.ExpectBody != "" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap body read at 1MB
		if !bytes.Contains(body, []byte(e.ExpectBody)) {
			st.Up = false
			st.Error = "body missing expected content"
		}
	}
	return st
}

// PollAll probes every endpoint concurrently, persists, updates metrics,
// tracks failures for paging, and broadcasts the fresh snapshot.
func (m *Monitor) PollAll(ctx context.Context) {
	var wg sync.WaitGroup
	results := make([]Status, len(m.endpoints))
	for i, e := range m.endpoints {
		wg.Add(1)
		go func(i int, e Endpoint) {
			defer wg.Done()
			results[i] = m.probe(ctx, e)
		}(i, e)
	}
	wg.Wait()

	m.mu.Lock()
	for _, st := range results {
		m.latest[st.Name] = st
		recordMetrics(st)
		m.store.Save(ctx, st)

		if st.Up {
			m.fails[st.Name] = 0
			continue
		}
		m.fails[st.Name]++
		// Fire once, on crossing the threshold, to avoid paging every cycle.
		if m.fails[st.Name] == failThreshold {
			sentry.CaptureMessage("endpoint down: " + st.Name + " (" + st.URL + ") — " +
				strconv.Itoa(failThreshold) + " consecutive failures")
		}
	}
	snapshot := make([]Status, 0, len(m.latest))
	for _, st := range m.latest {
		snapshot = append(snapshot, st)
	}
	m.mu.Unlock()

	if b, err := json.Marshal(snapshot); err == nil {
		m.hub.Broadcast(b)
	}
}

// Snapshot returns the current statuses (used by the /api/status fallback).
func (m *Monitor) Snapshot() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.latest))
	for _, st := range m.latest {
		out = append(out, st)
	}
	return out
}
