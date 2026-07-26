package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"statusping/internal/monitor"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx := context.Background()

	eps, err := monitor.LoadEndpoints(env("ENDPOINTS_FILE", "endpoints.json"))
	if err != nil {
		log.Fatalf("load endpoints: %v", err)
	}
	log.Printf("monitoring %d endpoints", len(eps))

	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		if err := sentry.Init(sentry.ClientOptions{Dsn: dsn}); err != nil {
			log.Printf("sentry init failed: %v", err)
		}
		defer sentry.Flush(2 * time.Second)
	}

	store, err := monitor.NewStore(ctx, env("MONGO_URI", "mongodb://localhost:27017"),
		env("REDIS_ADDR", "localhost:6379"))
	if err != nil {
		log.Fatalf("store: %v", err)
	}

	hub := monitor.NewHub()
	mon := monitor.NewMonitor(eps, store, hub)

	interval, _ := time.ParseDuration(env("POLL_INTERVAL", "15s"))
	go func() {
		mon.PollAll(ctx) // poll immediately, then on the tick
		for range time.Tick(interval) {
			mon.PollAll(ctx)
		}
	}()

	http.HandleFunc("/ws", hub.ServeWS)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mon.Snapshot())
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	addr := env("ADDR", ":8080")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
