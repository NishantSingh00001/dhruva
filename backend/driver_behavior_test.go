package main

import (
	"testing"
	"time"
)

func TestHeadingDelta(t *testing.T) {
	cases := []struct{ a, b, want float64 }{
		{10, 20, 10},
		{350, 10, 20}, // wraps across 0/360
		{0, 180, 180}, // opposite headings
		{90, 90, 0},
		{359, 1, 2},
	}
	for _, c := range cases {
		if got := headingDelta(c.a, c.b); got != c.want {
			t.Errorf("headingDelta(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestSeverityFor(t *testing.T) {
	if s := severityFor(5, 10); s != "low" {
		t.Errorf("want low, got %s", s)
	}
	if s := severityFor(14, 10); s != "medium" {
		t.Errorf("want medium, got %s", s)
	}
	if s := severityFor(25, 10); s != "high" {
		t.Errorf("want high, got %s", s)
	}
}

func TestDrivingStoreHarshBraking(t *testing.T) {
	d := newDrivingStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// 60 -> 20 km/h in 2s is a hard deceleration, well past the default 3.5 m/s^2 threshold.
	d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 60, Heading: 90, Timestamp: base})
	fired := d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 20, Heading: 90, Timestamp: base.Add(2 * time.Second)})
	if len(fired) != 1 || fired[0].Type != "harsh_braking" {
		t.Fatalf("expected one harsh_braking event, got %+v", fired)
	}
	score, ok := d.score("v1")
	if !ok || score.HarshBraking != 1 {
		t.Fatalf("expected score with 1 harsh braking, got %+v (ok=%v)", score, ok)
	}
	if score.Score != 97 { // 100 - 3*1
		t.Errorf("expected score 97, got %d", score.Score)
	}
}

func TestDrivingStoreHarshAcceleration(t *testing.T) {
	d := newDrivingStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 10, Heading: 90, Timestamp: base})
	fired := d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 40, Heading: 90, Timestamp: base.Add(2 * time.Second)})
	if len(fired) != 1 || fired[0].Type != "harsh_acceleration" {
		t.Fatalf("expected one harsh_acceleration event, got %+v", fired)
	}
}

func TestDrivingStoreOverspeed(t *testing.T) {
	d := newDrivingStore() // default SPEED_LIMIT_KMH is 80 when env unset
	fired := d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 95, Heading: 0, Timestamp: time.Now()})
	if len(fired) != 1 || fired[0].Type != "overspeed" {
		t.Fatalf("expected one overspeed event, got %+v", fired)
	}
}

func TestDrivingStoreSharpTurn(t *testing.T) {
	d := newDrivingStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 30, Heading: 0, Timestamp: base})
	fired := d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 30, Heading: 90, Timestamp: base.Add(1 * time.Second)})
	found := false
	for _, e := range fired {
		if e.Type == "sharp_turn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a sharp_turn event, got %+v", fired)
	}
}

func TestDrivingStoreNoFalsePositiveOnNormalDriving(t *testing.T) {
	d := newDrivingStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 40, Heading: 90, Timestamp: base})
	fired := d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 42, Heading: 91, Timestamp: base.Add(5 * time.Second)})
	if len(fired) != 0 {
		t.Fatalf("expected no events for gentle driving, got %+v", fired)
	}
}

func TestDrivingStoreStaleOrOutOfOrderIgnored(t *testing.T) {
	d := newDrivingStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 60, Heading: 90, Timestamp: base})
	// An out-of-order point (earlier timestamp) must not be treated as a new reading.
	fired := d.ingest(TelemetryPoint{VehicleID: "v1", Speed: 10, Heading: 90, Timestamp: base.Add(-5 * time.Second)})
	for _, e := range fired {
		if e.Type == "harsh_braking" {
			t.Fatalf("out-of-order point should not fire harsh_braking, got %+v", fired)
		}
	}
}

func TestIngestTelemetryHandler(t *testing.T) {
	a := newApp("")
	code, out, _ := call(t, a, "POST", "/telemetry", resp{
		"vehicle_id": "v1", "speed_kmh": 95, "heading_deg": 10, "timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	if code != 200 {
		t.Fatalf("POST /telemetry: want 200, got %d (%v)", code, out)
	}
	events, _ := out["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 fired event, got %v", out)
	}
}

func TestIngestTelemetryRequiresVehicleID(t *testing.T) {
	a := newApp("")
	code, _, _ := call(t, a, "POST", "/telemetry", resp{"speed_kmh": 50, "heading_deg": 0})
	if code != 400 {
		t.Fatalf("expected 400 for missing vehicle_id, got %d", code)
	}
}

func TestDriverScoreNotFoundForUnknownVehicle(t *testing.T) {
	a := newApp("")
	code, _, _ := call(t, a, "GET", "/drivers/does-not-exist/score", nil)
	if code != 404 {
		t.Fatalf("expected 404 for a vehicle with no telemetry, got %d", code)
	}
}

func TestDriverScoresAndEventsEndpoints(t *testing.T) {
	a := newApp("")
	call(t, a, "POST", "/telemetry", resp{"vehicle_id": "v1", "speed_kmh": 95, "heading_deg": 0, "timestamp": time.Now().UTC().Format(time.RFC3339)})

	code, out, _ := call(t, a, "GET", "/drivers/scores", nil)
	if code != 200 {
		t.Fatalf("GET /drivers/scores: want 200, got %d", code)
	}
	scores, _ := out["scores"].([]any)
	if len(scores) != 1 {
		t.Fatalf("expected 1 scored vehicle, got %v", out)
	}

	code, out, _ = call(t, a, "GET", "/drivers/v1/events", nil)
	if code != 200 {
		t.Fatalf("GET /drivers/v1/events: want 200, got %d", code)
	}
	events, _ := out["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 event for v1, got %v", out)
	}
}
