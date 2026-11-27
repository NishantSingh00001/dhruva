package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type resp = map[string]any

func call(t *testing.T, a *App, method, path string, body any) (int, resp, http.Header) {
	t.Helper()
	var buf bytes.Buffer
	if s, ok := body.(string); ok {
		buf.WriteString(s)
	} else if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
	var out resp
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Header()
}

var square = [][]float64{{12, 77}, {12, 78}, {13, 78}, {13, 77}, {12, 77}}

// seed creates one fence, one vehicle and an "both" rule; returns their ids.
func seed(t *testing.T, a *App) (fence, vehicle string) {
	t.Helper()
	_, f, _ := call(t, a, "POST", "/geofences", resp{"name": "Zone", "coordinates": square, "category": "restricted_zone"})
	_, v, _ := call(t, a, "POST", "/vehicles", resp{"vehicle_number": "KA-01", "driver_name": "D", "vehicle_type": "truck", "phone": "1"})
	fence, vehicle = f["id"].(string), v["id"].(string)
	if code, _, _ := call(t, a, "POST", "/alerts/configure", resp{"geofence_id": fence, "event_type": "both"}); code != 201 {
		t.Fatalf("configure alert: %d", code)
	}
	return
}

func move(t *testing.T, a *App, vehicle string, lat, lon float64, ts string) (int, resp) {
	code, out, _ := call(t, a, "POST", "/vehicles/location", resp{"vehicle_id": vehicle, "latitude": lat, "longitude": lon, "timestamp": ts})
	return code, out
}

func TestEntryExitFlowAndHistory(t *testing.T) {
	a := newApp("")
	fence, veh := seed(t, a)
	_, out := move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")
	if len(out["current_geofences"].([]any)) != 1 || out["time_ns"] == nil {
		t.Fatalf("expected inside + time_ns: %v", out)
	}
	move(t, a, veh, 20, 77.5, "2026-01-01T11:00:00Z")

	_, h, _ := call(t, a, "GET", "/violations/history?vehicle_id="+veh, nil)
	list := h["violations"].([]any)
	if len(list) != 2 || h["total_count"].(float64) != 2 {
		t.Fatalf("want 2 events, got %v", h)
	}
	first := list[0].(map[string]any)
	if first["event_type"] != "exit" || first["geofence_id"] != fence || first["vehicle_number"] != "KA-01" || first["latitude"] == nil {
		t.Fatalf("history should be flat, newest first: %v", first)
	}

	_, h, _ = call(t, a, "GET", "/violations/history?start_date=2026-01-01T10:30:00Z", nil)
	if len(h["violations"].([]any)) != 1 {
		t.Fatalf("start_date filter: %v", h)
	}
	_, h, _ = call(t, a, "GET", "/violations/history?limit=1&offset=1", nil)
	if len(h["violations"].([]any)) != 1 || h["total_count"].(float64) != 2 || h["violations"].([]any)[0].(map[string]any)["event_type"] != "entry" {
		t.Fatalf("pagination: %v", h)
	}
	if code, _, _ := call(t, a, "GET", "/violations/history?end_date=nope", nil); code != 400 {
		t.Fatalf("bad date should be 400, got %d", code)
	}
}

func TestOnlyMatchingRulesFireOnce(t *testing.T) {
	a := newApp("")
	fence, veh := seed(t, a)
	call(t, a, "POST", "/alerts/configure", resp{"geofence_id": fence, "event_type": "entry"}) // overlaps "both"
	move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")
	if n := len(a.state.Events); n != 1 {
		t.Fatalf("overlapping rules must fire a single event, got %d", n)
	}
}

func TestStaleLocationRejected(t *testing.T) {
	a := newApp("")
	_, veh := seed(t, a)
	move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")
	if code, _ := move(t, a, veh, 20, 77.5, "2026-01-01T09:00:00Z"); code != 409 {
		t.Fatalf("older timestamp should be 409, got %d", code)
	}
}

func TestAlertsAreEnrichedAndFiltered(t *testing.T) {
	a := newApp("")
	fence, _ := seed(t, a)
	_, out, _ := call(t, a, "GET", "/alerts?geofence_id="+fence, nil)
	al := out["alerts"].([]any)[0].(map[string]any)
	if al["geofence_name"] != "Zone" {
		t.Fatalf("alert should carry geofence_name: %v", al)
	}
	_, out, _ = call(t, a, "GET", "/alerts?geofence_id=other", nil)
	if len(out["alerts"].([]any)) != 0 {
		t.Fatal("filter ignored")
	}
}

func TestIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		if id := newID("evt"); seen[id] {
			t.Fatalf("duplicate id %s", id)
		} else {
			seen[id] = true
		}
	}
}

func TestValidationAndLimits(t *testing.T) {
	a := newApp("")
	cases := []struct {
		path string
		body any
		want int
	}{
		{"/geofences", resp{"name": "x", "coordinates": square, "category": "nope"}, 400},
		{"/vehicles", resp{"vehicle_number": "A"}, 400},
		{"/vehicles/location", resp{"vehicle_id": "x", "latitude": 99, "longitude": 0, "timestamp": "2026-01-01T00:00:00Z"}, 400},
		{"/alerts/configure", resp{"geofence_id": "missing", "event_type": "entry"}, 404},
		{"/vehicles", "{" + strings.Repeat("a", 2<<20), 400},
	}
	for _, c := range cases {
		if code, _, _ := call(t, a, "POST", c.path, c.body); code != c.want {
			t.Errorf("%s: want %d got %d", c.path, c.want, code)
		}
	}
}

func TestCORSDefaultsToWildcard(t *testing.T) {
	_, _, h := call(t, newApp(""), "OPTIONS", "/vehicles", nil)
	if h.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("got %q", h.Get("Access-Control-Allow-Origin"))
	}
}

func TestStatePersistsAcrossRestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state.json")
	a := newApp(file)
	_, veh := seed(t, a)
	move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")

	a.Flush() // snapshots are debounced; shutdown flushes too
	b := newApp(file)
	if _, out, _ := call(t, b, "GET", "/vehicles/location/"+veh, nil); len(out["current_geofences"].([]any)) != 1 {
		t.Fatalf("location lost after restart: %v", out)
	}
	if len(b.state.Events) != 1 {
		t.Fatal("events lost after restart")
	}
}

func TestSecurityHeaders(t *testing.T) {
	_, _, h := call(t, newApp(""), "GET", "/health", nil)
	if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") == "" {
		t.Fatalf("missing hardening headers: %v", h)
	}
}
