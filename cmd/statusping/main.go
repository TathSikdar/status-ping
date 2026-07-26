package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
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
	// Cancel everything on SIGINT/SIGTERM so `docker stop` shuts down cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		mon.PollAll(ctx) // poll immediately, then on the tick
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mon.PollAll(ctx)
			}
		}
	}()

	http.HandleFunc("/ws", hub.ServeWS)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mon.Snapshot())
	})
	http.HandleFunc("/api/history", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		hours := 24
		if v, err := strconv.Atoi(r.URL.Query().Get("hours")); err == nil && v > 0 {
			hours = v
		}
		since := time.Now().Add(-time.Duration(hours) * time.Hour)
		res, err := store.History(r.Context(), name, since, 300)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{Addr: env("ADDR", ":8080")}
	go func() {
		log.Printf("listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
}
