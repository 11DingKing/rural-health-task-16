package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// certupstream is a standalone upstream testing agency simulator.
// It receives dispatch requests from the gateway and returns mock
// test results with configurable pass rates and latency.

func main() {
	addr := flag.String("addr", ":52662", "listen address")
	name := flag.String("name", "Primary Testing Center", "agency name")
	failRate := flag.Float64("fail-rate", 0.1, "probability of returning a failed result")
	timeoutRate := flag.Float64("timeout-rate", 0.0, "probability of timing out")
	latency := flag.Duration("latency", 50*time.Millisecond, "simulated processing latency")
	flag.Parse()

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok", "agency": *name})
	})

	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ready", "agency": *name})
	})

	r.Post("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(*latency)

		if rnd.Float64() < *timeoutRate {
			time.Sleep(30 * time.Second)
			return
		}

		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)

		passed := rnd.Float64() >= *failRate
		score := 0.0
		if passed {
			score = 80 + rnd.Float64()*20
		} else {
			score = rnd.Float64() * 50
		}

		conclusion := "PASS"
		if !passed {
			conclusion = "FAIL"
		}

		resp := map[string]any{
			"dispatch_id": req["dispatch_id"],
			"passed":      passed,
			"score":       score,
			"conclusion":  conclusion,
			"detail":      fmt.Sprintf("Automated test by %s. Score: %.1f", *name, score),
			"tested_at":   time.Now().Format(time.RFC3339),
		}
		writeJSON(w, resp)
	})

	r.Get("/api/v1/agency", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"name":      *name,
			"addr":      *addr,
			"fail_rate": *failRate,
		})
	})

	fmt.Fprintf(os.Stderr, "upstream %s listening on %s (fail=%.1f%% timeout=%.1f%% latency=%s)\n",
		*name, *addr, *failRate*100, *timeoutRate*100, *latency)
	if err := http.ListenAndServe(*addr, r); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
