package main

import "testing"

func TestEveryLocationUpdateIsKeptAsMovementHistory(t *testing.T) {
	a := newApp("")
	_, veh := seed(t, a)
	move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")
	move(t, a, veh, 20, 77.5, "2026-01-01T11:00:00Z")
	move(t, a, veh, 12.6, 77.6, "2026-01-01T12:00:00Z")
	if code, _ := move(t, a, veh, 1, 1, "2025-01-01T00:00:00Z"); code != 409 { // stale update must not enter the trail
		t.Fatalf("stale update: %d", code)
	}

	code, out, _ := call(t, a, "GET", "/vehicles/location/"+veh+"/history", nil)
	list := out["locations"].([]any)
	if code != 200 || len(list) != 3 || out["total_count"].(float64) != 3 || out["vehicle_id"] != veh || out["time_ns"] == nil {
		t.Fatalf("history: %d %v", code, out)
	}
	newest := list[0].(map[string]any)
	if newest["latitude"].(float64) != 12.6 || newest["timestamp"] != "2026-01-01T12:00:00Z" {
		t.Fatalf("newest first expected: %v", newest)
	}

	_, page, _ := call(t, a, "GET", "/vehicles/location/"+veh+"/history?limit=1&offset=1", nil)
	if got := page["locations"].([]any)[0].(map[string]any)["latitude"].(float64); got != 20 || page["total_count"].(float64) != 3 {
		t.Fatalf("pagination: %v", page)
	}
	_, win, _ := call(t, a, "GET", "/vehicles/location/"+veh+"/history?start_date=2026-01-01T10:30:00Z&end_date=2026-01-01T11:30:00Z", nil)
	if len(win["locations"].([]any)) != 1 {
		t.Fatalf("date window: %v", win)
	}
	if code, _, _ := call(t, a, "GET", "/vehicles/location/nope/history", nil); code != 404 {
		t.Fatalf("unknown vehicle: %d", code)
	}
	if code, _, _ := call(t, a, "GET", "/vehicles/location/"+veh+"/history?start_date=bad", nil); code != 400 {
		t.Fatalf("bad date: %d", code)
	}
}

func TestMovementHistorySurvivesRestartInSnapshotMode(t *testing.T) {
	file := t.TempDir() + "/state.json"
	a := newApp(file)
	_, veh := seed(t, a)
	move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")
	a.Flush()
	b := newApp(file)
	_, out, _ := call(t, b, "GET", "/vehicles/location/"+veh+"/history", nil)
	if len(out["locations"].([]any)) != 1 {
		t.Fatalf("trail lost after restart: %v", out)
	}
}

func TestMovementHistoryInPostgres(t *testing.T) {
	a, url := pgApp(t)
	_, veh := seed(t, a)
	for i, ts := range []string{"2026-01-01T10:00:00Z", "2026-01-01T11:00:00Z", "2026-01-01T12:00:00Z"} {
		move(t, a, veh, 12.5+float64(i)*0.01, 77.5, ts)
	}
	_, out, _ := call(t, a, "GET", "/vehicles/location/"+veh+"/history?limit=2", nil)
	if len(out["locations"].([]any)) != 2 || out["total_count"].(float64) != 3 {
		t.Fatalf("db history: %v", out)
	}
	if len(a.state.Trail[veh]) != 0 {
		t.Fatal("with a database the trail must not also grow in memory")
	}
	b := secondApp(t, url)
	_, out, _ = call(t, b, "GET", "/vehicles/location/"+veh+"/history?start_date=2026-01-01T10:30:00Z", nil)
	if out["total_count"].(float64) != 2 {
		t.Fatalf("other instance / date filter: %v", out)
	}
}
