package main

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

// Both need a real PostgreSQL (TEST_DATABASE_URL); PostGIS tests also need the extension.

func secondApp(t *testing.T, url string) *App {
	b := newApp("")
	if err := b.useDB(context.Background(), url); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.db.close() })
	return b
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func TestTwoInstancesStayInSyncAndBothAlert(t *testing.T) {
	a, url := pgApp(t)
	b := secondApp(t, url)

	// config written on A appears on B
	fence, veh := seed(t, a)
	eventually(t, "B sees fence, vehicle and rule", func() bool {
		b.mu.RLock()
		defer b.mu.RUnlock()
		return len(b.state.Fences) == 1 && len(b.state.Vehicles) == 1 && len(b.state.Rules) == 1
	})

	subA, subB := a.hub.subscribe(), b.hub.subscribe()
	move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z") // handled by A only

	for name, ch := range map[string]chan Event{"A": subA, "B": subB} {
		select {
		case e := <-ch:
			if e.EventType != "entry" || e.FenceID != fence {
				t.Fatalf("%s got wrong event: %+v", name, e)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("instance %s never received the alert", name)
		}
	}
	select {
	case e := <-subA:
		t.Fatalf("A received a duplicate: %+v", e)
	case <-time.After(300 * time.Millisecond):
	}

	// B now knows where the vehicle is, so a move handled by B detects the exit
	eventually(t, "B sees location", func() bool {
		_, out, _ := call(t, b, "GET", "/vehicles/location/"+veh, nil)
		return len(out["current_geofences"].([]any)) == 1
	})
	move(t, b, veh, 20, 77.5, "2026-01-01T11:00:00Z")
	select {
	case e := <-subA:
		if e.EventType != "exit" {
			t.Fatalf("A should get B's exit, got %+v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("A never received the exit raised on B")
	}
}

func TestPostGISMatchesInMemoryRayCasting(t *testing.T) {
	a, _ := pgApp(t)
	if !a.db.postgis {
		t.Skip("PostGIS extension not available")
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 25; i++ {
		lat, lon := 10+rng.Float64()*5, 75+rng.Float64()*5
		h, w := 0.05+rng.Float64()*0.5, 0.05+rng.Float64()*0.5
		ring := [][]float64{{lat, lon}, {lat, lon + w}, {lat + h*0.6, lon + w*1.2}, {lat + h, lon + w}, {lat + h, lon}, {lat, lon}} // convex-ish, not a rectangle
		if code, _, _ := call(t, a, "POST", "/geofences", resp{"name": "g", "coordinates": ring, "category": "toll_zone"}); code != 201 {
			t.Fatalf("create: %d", code)
		}
	}
	a.db.sync()
	a.mu.RLock()
	fences := a.state.Fences
	a.mu.RUnlock()
	for i := 0; i < 400; i++ {
		lat, lon := 9.8+rng.Float64()*5.6, 74.8+rng.Float64()*5.6
		want := map[string]bool{}
		for id, f := range fences {
			if containsPoint(f.Coordinates, lat, lon) {
				want[id] = true
			}
		}
		got, err := a.db.containing(context.Background(), lat, lon)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("point (%f,%f): PostGIS %v vs ray casting %v", lat, lon, got, want)
		}
		for _, id := range got {
			if !want[id] {
				t.Fatalf("point (%f,%f): PostGIS says inside %s, ray casting disagrees", lat, lon, id)
			}
		}
	}
}

func TestLocationUpdateUsesPostGISAboveThreshold(t *testing.T) {
	a, _ := pgApp(t)
	if !a.db.postgis {
		t.Skip("PostGIS extension not available")
	}
	a.limits.postgisAbove = 0 // force the database path
	_, veh := seed(t, a)
	a.db.sync()
	_, out := move(t, a, veh, 12.5, 77.5, "2026-01-01T10:00:00Z")
	if len(out["current_geofences"].([]any)) != 1 {
		t.Fatalf("database containment path wrong: %v", out)
	}
	_, loc, _ := call(t, a, "GET", "/vehicles/location/"+veh, nil)
	if len(loc["current_geofences"].([]any)) != 1 {
		t.Fatalf("getLocation via PostGIS wrong: %v", loc)
	}
}
