package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxBody       = 1 << 20
	maxPolyPoints = 1000
)

var categories = []string{"delivery_zone", "restricted_zone", "toll_zone", "customer_area"}

func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}

type ctxStart struct{}

// routes wires endpoints, CORS and per-request timing (time_ns).
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { reply(w, r, 200, map[string]any{"status": "ok"}) })
	mux.HandleFunc("POST /geofences", a.createFence)
	mux.HandleFunc("GET /geofences", a.listFences)
	mux.HandleFunc("POST /vehicles", a.createVehicle)
	mux.HandleFunc("GET /vehicles", a.listVehicles)
	mux.HandleFunc("POST /vehicles/location", a.updateLocation)
	mux.HandleFunc("GET /vehicles/location/{id}", a.getLocation)
	mux.HandleFunc("GET /vehicles/location/{id}/history", a.locationHistory)
	mux.HandleFunc("POST /alerts/configure", a.configureAlert)
	mux.HandleFunc("GET /alerts", a.listAlerts)
	mux.HandleFunc("DELETE /alerts/{id}", a.deleteAlert)
	mux.HandleFunc("GET /violations/history", a.history)
	mux.HandleFunc("GET /ws/alerts", a.stream)
	mux.HandleFunc("POST /telemetry", a.ingestTelemetry)
	mux.HandleFunc("GET /drivers/scores", a.driverScores)
	mux.HandleFunc("GET /drivers/{id}/score", a.driverScore)
	mux.HandleFunc("GET /drivers/{id}/events", a.driverEvents)

	origin := os.Getenv("CORS_ORIGIN")
	if origin == "" {
		origin = "*"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
		h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		r = r.WithContext(contextWithStart(r))
		if r.URL.Path != "/health" {
			// throttle first so key guessing is rate limited too
			if a.limiter != nil {
				if ok, wait := a.limiter.allow(a.clientKey(r)); !ok {
					w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
					fail(w, r, 429, "rate limit exceeded, slow down")
					return
				}
			}
			if !a.authorized(r) {
				fail(w, r, 401, "missing or invalid API key")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

// authorized checks the API key in constant time. EventSource cannot set headers,
// so only the live stream also accepts ?key=.
func (a *App) authorized(r *http.Request) bool {
	if a.apiKey == "" {
		return true
	}
	got := r.Header.Get("X-API-Key")
	if got == "" {
		got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if got == "" && r.URL.Path == "/ws/alerts" {
		got = r.URL.Query().Get("key")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(a.apiKey)) == 1
}

// clientKey identifies the caller for rate limiting; forwarded headers count only behind a trusted proxy.
func (a *App) clientKey(r *http.Request) string {
	if a.trustProxy {
		if parts := strings.Split(r.Header.Get("X-Forwarded-For"), ","); parts[0] != "" {
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// reply writes v (a struct or map) as JSON with time_ns added.
func reply(w http.ResponseWriter, r *http.Request, status int, v any) {
	body := map[string]any{}
	if raw, err := json.Marshal(v); err == nil {
		json.Unmarshal(raw, &body)
	}
	start, _ := r.Context().Value(ctxStart{}).(time.Time)
	body["time_ns"] = strconv.FormatInt(time.Since(start).Nanoseconds(), 10)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, r *http.Request, status int, msg string) {
	reply(w, r, status, map[string]any{"error": msg})
}

// tooLong reports whether any string exceeds max bytes.
func tooLong(max int, values ...string) bool {
	for _, v := range values {
		if len(v) > max {
			return true
		}
	}
	return false
}

// paginate applies optional ?limit (1..500) and ?offset to a list. With no limit the whole list is returned,
// so existing clients keep working; total is always the unpaginated size.
func paginate[T any](items []T, q url.Values) ([]T, int) {
	total := len(items)
	offset, _ := strconv.Atoi(q.Get("offset"))
	offset = max(0, min(offset, total))
	items = items[offset:]
	if limit, _ := strconv.Atoi(q.Get("limit")); limit > 0 {
		items = items[:min(limit, 500, len(items))]
	}
	return items, total
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(v) == nil
}

func (a *App) createFence(w http.ResponseWriter, r *http.Request) {
	var in Geofence
	if !decode(w, r, &in) || strings.TrimSpace(in.Name) == "" || !validPolygon(in.Coordinates) || len(in.Coordinates) > maxPolyPoints || tooLong(120, in.Name) || tooLong(500, in.Description) || !oneOf(in.Category, categories...) {
		fail(w, r, 400, "Provide name, a closed polygon of [lat, lon] pairs (min 4 points), and a supported category")
		return
	}
	in.ID, in.CreatedAt = newID("geo"), time.Now().UTC()
	a.mu.Lock()
	if len(a.state.Fences) >= a.limits.fences {
		a.mu.Unlock()
		fail(w, r, 429, "geofence limit reached")
		return
	}
	a.state.Fences[in.ID] = in
	a.record(Change{Fence: &in})
	a.save()
	a.mu.Unlock()
	reply(w, r, 201, map[string]any{"id": in.ID, "name": in.Name, "status": "active"})
}

func (a *App) listFences(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")
	a.mu.RLock()
	out := []Geofence{}
	for _, f := range a.state.Fences {
		if cat == "" || f.Category == cat {
			out = append(out, f)
		}
	}
	a.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	out, total := paginate(out, r.URL.Query())
	reply(w, r, 200, map[string]any{"geofences": out, "total_count": total})
}

func (a *App) createVehicle(w http.ResponseWriter, r *http.Request) {
	var in Vehicle
	if !decode(w, r, &in) {
		fail(w, r, 400, "Invalid JSON body")
		return
	}
	in.Number = strings.TrimSpace(in.Number)
	if in.Number == "" || in.Driver == "" || in.Type == "" || in.Phone == "" || tooLong(32, in.Number, in.Type, in.Phone) || tooLong(100, in.Driver) {
		fail(w, r, 400, "vehicle_number, driver_name, vehicle_type and phone are required (and must be short)")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.state.Vehicles) >= a.limits.vehicles {
		fail(w, r, 429, "vehicle limit reached")
		return
	}
	for _, v := range a.state.Vehicles {
		if strings.EqualFold(v.Number, in.Number) {
			fail(w, r, 409, "Vehicle number already exists")
			return
		}
	}
	in.ID, in.Status, in.CreatedAt, in.Location = newID("veh"), "active", time.Now().UTC(), nil
	a.state.Vehicles[in.ID] = in
	a.record(Change{Vehicle: &in})
	a.save()
	reply(w, r, 201, map[string]any{"id": in.ID, "vehicle_number": in.Number, "status": in.Status})
}

func (a *App) listVehicles(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	out := []Vehicle{}
	for _, v := range a.state.Vehicles {
		out = append(out, v)
	}
	a.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	out, total := paginate(out, r.URL.Query())
	reply(w, r, 200, map[string]any{"vehicles": out, "total_count": total})
}

func (a *App) updateLocation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		VehicleID string    `json:"vehicle_id"`
		Latitude  float64   `json:"latitude"`
		Longitude float64   `json:"longitude"`
		Timestamp time.Time `json:"timestamp"`
	}
	if !decode(w, r, &in) || !validLocationInput(in.VehicleID, in.Latitude, in.Longitude, in.Timestamp) {
		fail(w, r, 400, "vehicle_id, latitude (-90..90), longitude (-180..180) and an ISO 8601 timestamp are required")
		return
	}
	current, _, err := a.applyLocationUpdate(r.Context(), in.VehicleID, in.Latitude, in.Longitude, in.Timestamp, false)
	switch err {
	case errVehicleNotFound:
		fail(w, r, 404, "vehicle not found")
		return
	case errStaleTimestamp:
		fail(w, r, 409, "timestamp is older than the vehicle's latest location")
		return
	}
	reply(w, r, 200, map[string]any{"vehicle_id": in.VehicleID, "location_updated": true, "current_geofences": current})
}

func validLocationInput(vehicleID string, lat, lon float64, ts time.Time) bool {
	return vehicleID != "" && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 && !ts.IsZero()
}

var (
	errVehicleNotFound = fmt.Errorf("vehicle not found")
	errStaleTimestamp  = fmt.Errorf("timestamp is older than the vehicle's latest location")
)

// applyLocationUpdate is the shared core behind both the REST /vehicles/location
// endpoint and the Kafka/Condense telemetry consumer: it records the new position,
// evaluates every geofence, fires and broadcasts any entry/exit events, and persists
// state. If autoCreate is true and the vehicle id is unknown, a minimal vehicle record
// is created on the fly (telemetry streams reference their own device ids, which may
// arrive before any REST-registered vehicle with that id exists).
func (a *App) applyLocationUpdate(ctx context.Context, vehicleID string, lat, lon float64, ts time.Time, autoCreate bool) (current []map[string]string, fired []Event, err error) {
	inside := a.insideSet(ctx, lat, lon) // before locking: may query PostGIS
	a.mu.Lock()
	v, ok := a.state.Vehicles[vehicleID]
	if !ok {
		if !autoCreate {
			a.mu.Unlock()
			return nil, nil, errVehicleNotFound
		}
		v = Vehicle{ID: vehicleID, Number: vehicleID, Driver: "unknown", Type: "unknown", Status: "active", CreatedAt: time.Now().UTC()}
	}
	if v.Location != nil && ts.Before(v.Location.Timestamp) {
		a.mu.Unlock()
		return nil, nil, errStaleTimestamp
	}
	v.Location = &Location{lat, lon, ts}
	a.state.Vehicles[v.ID] = v
	a.appendTrail(v.ID, *v.Location)

	current, now := []map[string]string{}, map[string]bool{}
	for fid, f := range a.state.Fences {
		inFence := inside[fid]
		if inFence {
			now[fid] = true
			current = append(current, map[string]string{"geofence_id": fid, "geofence_name": f.Name, "status": "inside"})
		}
		if a.state.Inside[v.ID][fid] == inFence {
			continue
		}
		kind := "exit"
		if inFence {
			kind = "entry"
		}
		if a.ruleMatches(fid, v.ID, kind) {
			fired = append(fired, Event{
				ID: newID("viol"), EventID: newID("evt"), EventType: kind, VehicleID: v.ID, VehicleNum: v.Number, Driver: v.Driver,
				FenceID: fid, FenceName: f.Name, Category: f.Category, Latitude: lat, Longitude: lon, Timestamp: ts,
			})
		}
	}
	a.state.Inside[v.ID] = now
	a.state.Events = append(a.state.Events, fired...)
	if over := len(a.state.Events) - maxEvents; over > 0 {
		a.state.Events = a.state.Events[over:]
	}
	a.record(Change{Vehicle: &v, Events: fired, Trail: true})
	a.save()
	a.mu.Unlock()

	go a.hub.broadcast(fired) // async: never delays the caller
	return current, fired, nil
}

// ruleMatches reports whether any rule wants this transition; caller holds a.mu.
func (a *App) ruleMatches(fence, vehicle, kind string) bool {
	for _, rule := range a.state.Rules {
		if rule.FenceID == fence && (rule.VehicleID == "" || rule.VehicleID == vehicle) && (rule.EventType == "both" || rule.EventType == kind) {
			return true
		}
	}
	return false
}

func (a *App) getLocation(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	v, ok := a.state.Vehicles[r.PathValue("id")]
	a.mu.RUnlock()
	if !ok || v.Location == nil {
		fail(w, r, 404, "vehicle or location not found")
		return
	}
	inside := a.insideSet(r.Context(), v.Location.Latitude, v.Location.Longitude)
	fences := []map[string]string{}
	a.mu.RLock()
	for fid := range inside {
		if f, ok := a.state.Fences[fid]; ok {
			fences = append(fences, map[string]string{"geofence_id": fid, "geofence_name": f.Name, "category": f.Category})
		}
	}
	a.mu.RUnlock()
	reply(w, r, 200, map[string]any{"vehicle_id": v.ID, "vehicle_number": v.Number, "current_location": v.Location, "current_geofences": fences})
}

func (a *App) configureAlert(w http.ResponseWriter, r *http.Request) {
	var in Rule
	if !decode(w, r, &in) || in.FenceID == "" || !oneOf(in.EventType, "entry", "exit", "both") {
		fail(w, r, 400, "geofence_id and event_type (entry, exit or both) are required")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.state.Fences[in.FenceID]; !ok {
		fail(w, r, 404, "geofence not found")
		return
	}
	if _, ok := a.state.Vehicles[in.VehicleID]; in.VehicleID != "" && !ok {
		fail(w, r, 404, "vehicle not found")
		return
	}
	in.ID, in.Status, in.CreatedAt = newID("alert"), "active", time.Now().UTC()
	in.FenceName, in.VehicleNum = "", ""
	a.state.Rules[in.ID] = in
	a.record(Change{Rule: &in})
	a.save()
	reply(w, r, 201, in)
}

func (a *App) listAlerts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a.mu.RLock()
	out := []Rule{}
	for _, rule := range a.state.Rules {
		if (q.Get("geofence_id") == "" || rule.FenceID == q.Get("geofence_id")) && (q.Get("vehicle_id") == "" || rule.VehicleID == q.Get("vehicle_id")) {
			rule.FenceName = a.state.Fences[rule.FenceID].Name
			rule.VehicleNum = a.state.Vehicles[rule.VehicleID].Number
			out = append(out, rule)
		}
	}
	a.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	out, total := paginate(out, r.URL.Query())
	reply(w, r, 200, map[string]any{"alerts": out, "total_count": total})
}

// deleteAlert removes a rule; events stop firing for it immediately.
func (a *App) deleteAlert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.mu.Lock()
	if _, ok := a.state.Rules[id]; !ok {
		a.mu.Unlock()
		fail(w, r, 404, "alert rule not found")
		return
	}
	delete(a.state.Rules, id)
	a.record(Change{DeleteRule: id})
	a.save()
	a.mu.Unlock()
	reply(w, r, 200, map[string]any{"alert_id": id, "status": "deleted"})
}

// locationHistory returns a vehicle's past positions, newest first, with date filters and limit/offset paging.
func (a *App) locationHistory(w http.ResponseWriter, r *http.Request) {
	id, q := r.PathValue("id"), r.URL.Query()
	start, hasStart, e1 := parseDate(q.Get("start_date"), "start_date")
	end, hasEnd, e2 := parseDate(q.Get("end_date"), "end_date")
	if e1 != nil || e2 != nil {
		fail(w, r, 400, "start_date and end_date must be ISO 8601, e.g. 2026-01-01T00:00:00Z")
		return
	}
	a.mu.RLock()
	_, ok := a.state.Vehicles[id]
	a.mu.RUnlock()
	if !ok {
		fail(w, r, 404, "vehicle not found")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	offset = max(0, offset)

	var page []Location
	var total int
	if a.db != nil {
		a.db.sync()
		var from, to *time.Time
		if hasStart {
			from = &start
		}
		if hasEnd {
			to = &end
		}
		var err error
		if page, total, err = a.db.trail(r.Context(), id, from, to, limit, offset); err != nil {
			fail(w, r, 500, "movement history query failed")
			return
		}
	} else {
		a.mu.RLock()
		trail := a.state.Trail[id]
		matched := []Location{}
		for i := len(trail) - 1; i >= 0; i-- {
			if (!hasStart || !trail[i].Timestamp.Before(start)) && (!hasEnd || !trail[i].Timestamp.After(end)) {
				matched = append(matched, trail[i])
			}
		}
		a.mu.RUnlock()
		total = len(matched)
		offset = min(offset, total)
		page = matched[offset:min(offset+limit, total)]
	}
	reply(w, r, 200, map[string]any{"vehicle_id": id, "locations": page, "total_count": total})
}

func parseDate(q, name string) (t time.Time, ok bool, err error) {
	if s := q; s != "" {
		t, err = time.Parse(time.RFC3339, s)
		return t, true, err
	}
	return
}

// history returns events newest first, filtered by vehicle/geofence/date and paginated by limit/offset.
func (a *App) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start, hasStart, e1 := parseDate(q.Get("start_date"), "start_date")
	end, hasEnd, e2 := parseDate(q.Get("end_date"), "end_date")
	if e1 != nil || e2 != nil {
		fail(w, r, 400, "start_date and end_date must be ISO 8601, e.g. 2026-01-01T00:00:00Z")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	offset = max(0, offset)
	if a.db != nil {
		a.db.sync() // read-your-writes: wait for queued inserts
		var from, to *time.Time
		if hasStart {
			from = &start
		}
		if hasEnd {
			to = &end
		}
		page, total, err := a.db.history(r.Context(), q.Get("vehicle_id"), q.Get("geofence_id"), from, to, limit, offset)
		if err != nil {
			fail(w, r, 500, "history query failed")
			return
		}
		reply(w, r, 200, map[string]any{"violations": page, "total_count": total})
		return
	}
	a.mu.RLock()
	matched := []Event{}
	for i := len(a.state.Events) - 1; i >= 0; i-- {
		e := a.state.Events[i]
		if (q.Get("vehicle_id") == "" || e.VehicleID == q.Get("vehicle_id")) && (q.Get("geofence_id") == "" || e.FenceID == q.Get("geofence_id")) &&
			(!hasStart || !e.Timestamp.Before(start)) && (!hasEnd || !e.Timestamp.After(end)) {
			matched = append(matched, e)
		}
	}
	a.mu.RUnlock()
	offset = min(offset, len(matched))
	page := matched[offset:min(offset+limit, len(matched))]
	reply(w, r, 200, map[string]any{"violations": page, "total_count": len(matched)})
}

// stream is the real-time channel (Server-Sent Events) at /ws/alerts.
func (a *App) stream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	ch := a.hub.subscribeMax(a.limits.streams)
	if ch == nil {
		http.Error(w, "too many live connections", http.StatusServiceUnavailable)
		return
	}
	defer a.hub.unsubscribe(ch)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, ": connected\n\n")
	f.Flush()
	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			fmt.Fprint(w, ": ping\n\n")
		case e := <-ch:
			data, _ := json.Marshal(e.Alert())
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		f.Flush()
	}
}
