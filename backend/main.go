package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func contextWithStart(r *http.Request) context.Context {
	return context.WithValue(r.Context(), ctxStart{}, time.Now())
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":" + envOr("PORT", "8080")
	}
	app := newApp(os.Getenv("DATA_FILE"))
	if url := os.Getenv("DATABASE_URL"); url != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := app.useDB(ctx, url); err != nil {
			log.Fatalf("database: %v", err)
		}
		log.Print("persisting to PostgreSQL")
	}
	app.apiKey = os.Getenv("API_KEY")
	app.trustProxy = os.Getenv("TRUST_PROXY") == "1"
	if rps := envFloat("RATE_RPS", 20); rps > 0 {
		app.limiter = newLimiter(rps, envFloat("RATE_BURST", 40), time.Now)
	}
	if app.apiKey == "" {
		log.Print("WARNING: API_KEY is not set; the API is open to anyone")
	}
	srv := &http.Server{Addr: addr, Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}

	kafkaCtx, stopKafka := context.WithCancel(context.Background())
	if cfg, enabled := kafkaConfigFromEnv(); enabled {
		go runKafkaConsumer(kafkaCtx, app, cfg)
	}

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		stopKafka()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		app.Flush()
		if app.db != nil {
			app.db.close()
		}
	}()
	log.Printf("geofence API listening on %s", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return v
	}
	return def
}
