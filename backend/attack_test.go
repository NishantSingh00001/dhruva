package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Attacker mindset: every case below is something a hostile or sloppy client could send.

func TestMalformedAndHostileInputIsRejected(t *testing.T) {
	a := newApp("")
	long := strings.Repeat("x", 5000)
	bigRing := make([][]float64, 1500)
	for i := range bigRing {
		bigRing[i] = []float64{12 + float64(i%2)*0.001, 77 + float64(i)*0.0001}
	}
	bigRing[len(bigRing)-1] = bigRing[0]
	cases := []struct {
		name, path string
		body       any
	}{
		{"wrong type for latitude", "/vehicles/location", `{"vehicle_id":"x","latitude":"12","longitude":1,"timestamp":"2026-01-01T00:00:00Z"}`},
		{"bare array", "/geofences", `[]`},
		{"null body", "/vehicles", `null`},
		{"empty body", "/alerts/configure", ``},
		{"giant name", "/geofences", resp{"name": long, "coordinates": square, "category": "toll_zone"}},
		{"giant polygon", "/geofences", resp{"name": "big", "coordinates": bigRing, "category": "toll_zone"}},
		{"giant vehicle field", "/vehicles", resp{"vehicle_number": long, "driver_name": "d", "vehicle_type": "t", "phone": "1"}},
		{"whitespace-only name", "/geofences", resp{"name": "   ", "coordinates": square, "category": "toll_zone"}},
	}
	for _, c := range cases {
		if code, _, _ := call(t, a, "POST", c.path, c.body); code != 400 {
			t.Errorf("%s: want 400, got %d", c.name, code)
		}
	}
	if code, _, _ := call(t, a, "DELETE", "/vehicles", nil); code != 405 {
		t.Errorf("DELETE should be 405, got %d", code)
	}
}

func TestVehicleNumbersAreUniqueIgnoringCase(t *testing.T) {
	a := newApp("")
	body := func(n string) resp {
		return resp{"vehicle_number": n, "driver_name": "d", "vehicle_type": "t", "phone": "1"}
	}
	call(t, a, "POST", "/vehicles", body("KA-01-AB-1234"))
	if code, _, _ := call(t, a, "POST", "/vehicles", body(" ka-01-ab-1234 ")); code != 409 {
		t.Fatalf("case/space variant should collide, got %d", code)
	}
}

func TestResourceCaps(t *testing.T) {
	a := newApp("")
	a.limits.fences, a.limits.vehicles = 2, 2
	for i := 0; i < 2; i++ {
		call(t, a, "POST", "/geofences", resp{"name": "f", "coordinates": square, "category": "toll_zone"})
		call(t, a, "POST", "/vehicles", resp{"vehicle_number": fmt.Sprint(i), "driver_name": "d", "vehicle_type": "t", "phone": "1"})
	}
	if code, _, _ := call(t, a, "POST", "/geofences", resp{"name": "f", "coordinates": square, "category": "toll_zone"}); code != 429 {
		t.Errorf("fence cap: got %d", code)
	}
	if code, _, _ := call(t, a, "POST", "/vehicles", resp{"vehicle_number": "z", "driver_name": "d", "vehicle_type": "t", "phone": "1"}); code != 429 {
		t.Errorf("vehicle cap: got %d", code)
	}
}

func TestStreamRejectsTooManySubscribers(t *testing.T) {
	a := newApp("")
	a.limits.streams = 1
	srv := httptest.NewServer(a.routes())
	defer srv.Close()
	first, err := http.Get(srv.URL + "/ws/alerts")
	if err != nil || first.StatusCode != 200 {
		t.Fatalf("first stream: %v", err)
	}
	defer first.Body.Close()
	second, err := http.Get(srv.URL + "/ws/alerts")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()
	if second.StatusCode != 503 {
		t.Fatalf("second stream should be 503, got %d", second.StatusCode)
	}
}

// Many virtual clients at once (goroutines, not real devices). Run with -race.
func TestConcurrentFleetLoad(t *testing.T) { runLoad(t, newApp("")) }

// Same load with disk snapshots on: shows what persistence costs per request.
func TestConcurrentFleetLoadWithPersistence(t *testing.T) {
	runLoad(t, newApp(filepath.Join(t.TempDir(), "state.json")))
}

func runLoad(t *testing.T, a *App) {
	seed(t, a)
	const clients, hops = 100, 40
	ids := make([]string, clients)
	for i := range ids {
		_, v, _ := call(t, a, "POST", "/vehicles", resp{"vehicle_number": fmt.Sprintf("V%03d", i), "driver_name": "d", "vehicle_type": "truck", "phone": "1"})
		ids[i] = v["id"].(string)
	}
	// subscribers listening while the load runs
	var got atomic.Int64
	var subs sync.WaitGroup
	stop := make(chan struct{})
	for s := 0; s < 5; s++ {
		ch := a.hub.subscribe()
		subs.Add(1)
		go func() {
			defer subs.Done()
			defer a.hub.unsubscribe(ch)
			for {
				select {
				case <-ch:
					got.Add(1)
				case <-stop:
					return
				}
			}
		}()
	}
	start := time.Now()
	var wg sync.WaitGroup
	var failures atomic.Int64
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for h := 0; h < hops; h++ {
				lat := 12.5 // inside the square on even hops, outside on odd
				if h%2 == 1 {
					lat = 20
				}
				code, _ := move(t, a, id, lat, 77.5, base.Add(time.Duration(h)*time.Second).Format(time.RFC3339))
				if code != 200 {
					failures.Add(1)
				}
			}
		}(id)
	}
	wg.Wait()
	elapsed := time.Since(start)
	time.Sleep(200 * time.Millisecond) // let async broadcasts drain
	close(stop)
	subs.Wait()

	// seed's own vehicle never moved, so every event comes from the load: one entry+exit per hop pair.
	if a.file != "" && elapsed > 2*time.Second {
		t.Fatalf("persistence made requests too slow: %v for %d requests", elapsed, clients*hops)
	}
	wantEvents := clients * hops
	if failures.Load() != 0 {
		t.Fatalf("%d requests failed", failures.Load())
	}
	if n := len(a.state.Events); n != wantEvents {
		t.Fatalf("events: want %d got %d (lost or duplicated under concurrency)", wantEvents, n)
	}
	seen := map[string]bool{}
	for _, e := range a.state.Events {
		if seen[e.EventID] {
			t.Fatalf("duplicate event id %s", e.EventID)
		}
		seen[e.EventID] = true
	}
	t.Logf("%d virtual clients x %d updates = %d requests in %v (%.0f req/s), %d events, %d stream deliveries",
		clients, hops, clients*hops, elapsed.Round(time.Millisecond), float64(clients*hops)/elapsed.Seconds(), wantEvents, got.Load())
}
