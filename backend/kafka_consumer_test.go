package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTelemetryMessageFieldAliases(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantID  string
		wantLat float64
		wantLon float64
	}{
		{"canonical", `{"vehicle_id":"v1","latitude":12.9,"longitude":77.6,"timestamp":"2026-01-01T00:00:00Z"}`, "v1", 12.9, 77.6},
		{"camelCase id and short coords", `{"vehicleId":"v2","lat":1.5,"lon":2.5,"ts":"2026-01-01T00:00:00Z"}`, "v2", 1.5, 2.5},
		{"bare id and lng", `{"id":"v3","latitude":3.3,"lng":4.4,"time":"2026-01-01T00:00:00Z"}`, "v3", 3.3, 4.4},
		{"epoch millis timestamp", `{"vehicle_id":"v4","latitude":5,"longitude":6,"timestamp_ms":1735689600000}`, "v4", 5, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var tm telemetryMessage
			if err := json.Unmarshal([]byte(c.payload), &tm); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := tm.vehicleID(); got != c.wantID {
				t.Errorf("vehicleID() = %q, want %q", got, c.wantID)
			}
			lat, lon := tm.coords()
			if lat != c.wantLat || lon != c.wantLon {
				t.Errorf("coords() = (%v, %v), want (%v, %v)", lat, lon, c.wantLat, c.wantLon)
			}
			if _, ok := tm.when(); !ok {
				t.Errorf("when() reported no timestamp for %q", c.payload)
			}
		})
	}
}

func TestTelemetryMessageWhenEpochSeconds(t *testing.T) {
	var tm telemetryMessage
	if err := json.Unmarshal([]byte(`{"vehicle_id":"v1","latitude":1,"longitude":2,"ts":"1735689600"}`), &tm); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, ok := tm.when()
	if !ok {
		t.Fatal("when() reported no timestamp")
	}
	want := time.UnixMilli(1735689600).UTC()
	if !got.Equal(want) {
		t.Errorf("when() = %v, want %v", got, want)
	}
}

func TestKafkaConfigFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "")
	if _, enabled := kafkaConfigFromEnv(); enabled {
		t.Fatal("expected consumer disabled when KAFKA_BROKERS is unset")
	}
}

func TestKafkaConfigFromEnv(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "b1:9092, b2:9092")
	t.Setenv("KAFKA_USERNAME", "user")
	t.Setenv("KAFKA_PASSWORD", "pass")
	cfg, enabled := kafkaConfigFromEnv()
	if !enabled {
		t.Fatal("expected consumer enabled when KAFKA_BROKERS is set")
	}
	if len(cfg.Brokers) != 2 || cfg.Brokers[0] != "b1:9092" || cfg.Brokers[1] != "b2:9092" {
		t.Errorf("Brokers = %v", cfg.Brokers)
	}
	if !cfg.TLS {
		t.Error("expected TLS auto-enabled once a username is set")
	}
	if cfg.Topic != "vehicle-telemetry" || cfg.GroupID != "dhruva-geofencing" {
		t.Errorf("unexpected defaults: topic=%q group=%q", cfg.Topic, cfg.GroupID)
	}
}
