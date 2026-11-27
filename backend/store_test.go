package main

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration test against a real PostgreSQL. Skipped unless TEST_DATABASE_URL is set, e.g.
//
//	TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/geofence_test go test ./...
func pgApp(t *testing.T) (*App, string) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	pool.Exec(ctx, `DROP TABLE IF EXISTS violations, alert_rules, vehicles, geofences CASCADE`)
	pool.Close()
	a := newApp("")
	if err := a.useDB(ctx, url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.db.close() })
	return a, url
}

func TestPostgresRoundTripHistoryAndRestart(t *testing.T) {
	a, url := pgApp(t)
	fence, veh := seed(t, a)
	move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")
	move(t, a, veh, 20, 77.5, "2026-01-01T11:00:00Z")
	move(t, a, veh, 12.5, 77.5, "2026-01-02T09:00:00Z")

	_, h, _ := call(t, a, "GET", "/violations/history?vehicle_id="+veh+"&limit=2&offset=1", nil)
	if h["total_count"].(float64) != 3 || len(h["violations"].([]any)) != 2 {
		t.Fatalf("pagination from db: %v", h)
	}
	_, h, _ = call(t, a, "GET", "/violations/history?geofence_id="+fence+"&start_date=2026-01-01T10:30:00Z&end_date=2026-01-01T23:00:00Z", nil)
	if h["total_count"].(float64) != 1 || h["violations"].([]any)[0].(map[string]any)["event_type"] != "exit" {
		t.Fatalf("date window from db: %v", h)
	}

	a.db.close()
	b := newApp("")
	if err := b.useDB(context.Background(), url); err != nil {
		t.Fatal(err)
	}
	a.db = b.db // so Cleanup closes the live pool
	_, loc, _ := call(t, b, "GET", "/vehicles/location/"+veh, nil)
	if len(loc["current_geofences"].([]any)) != 1 {
		t.Fatalf("vehicle location/fence lost after restart: %v", loc)
	}
	// already inside after restart -> same point again must not raise a duplicate entry
	move(t, b, veh, 12.6, 77.6, "2026-01-02T09:05:00Z")
	_, h, _ = call(t, b, "GET", "/violations/history", nil)
	if h["total_count"].(float64) != 3 {
		t.Fatalf("restart re-fired an event: %v", h)
	}
	if len(b.state.Rules) != 1 {
		t.Fatal("rules not reloaded")
	}
}
