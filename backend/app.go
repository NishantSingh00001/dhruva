package main

import (
	"context"
	"errors"

	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Geofence struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Coordinates [][]float64 `json:"coordinates"`
	Category    string      `json:"category"`
	CreatedAt   time.Time   `json:"created_at"`
}

type Location struct {
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Timestamp time.Time `json:"timestamp"`
}

type Vehicle struct {
	ID        string    `json:"id"`
	Number    string    `json:"vehicle_number"`
	Driver    string    `json:"driver_name"`
	Type      string    `json:"vehicle_type"`
	Phone     string    `json:"phone"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Location  *Location `json:"current_location,omitempty"`
}

type Rule struct {
	ID         string    `json:"alert_id"`
	FenceID    string    `json:"geofence_id"`
	FenceName  string    `json:"geofence_name,omitempty"`
	VehicleID  string    `json:"vehicle_id,omitempty"`
	VehicleNum string    `json:"vehicle_number,omitempty"`
	EventType  string    `json:"event_type"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// Event is a stored entry/exit record (flat, as returned by /violations/history).
type Event struct {
	ID         string    `json:"id"`
	EventID    string    `json:"event_id"`
	EventType  string    `json:"event_type"`
	VehicleID  string    `json:"vehicle_id"`
	VehicleNum string    `json:"vehicle_number"`
	Driver     string    `json:"driver_name"`
	FenceID    string    `json:"geofence_id"`
	FenceName  string    `json:"geofence_name"`
	Category   string    `json:"category"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	Timestamp  time.Time `json:"timestamp"`
}

// Alert is the nested real-time message shape from the spec.
func (e Event) Alert() map[string]any {
	return map[string]any{
		"event_id": e.EventID, "event_type": e.EventType, "timestamp": e.Timestamp,
		"vehicle":  map[string]string{"vehicle_id": e.VehicleID, "vehicle_number": e.VehicleNum, "driver_name": e.Driver},
		"geofence": map[string]string{"geofence_id": e.FenceID, "geofence_name": e.FenceName, "category": e.Category},
		"location": map[string]float64{"latitude": e.Latitude, "longitude": e.Longitude},
	}
}

// State is everything persisted to disk.
type State struct {
	Fences   map[string]Geofence
	Vehicles map[string]Vehicle
	Rules    map[string]Rule
	Inside   map[string]map[string]bool // vehicle id -> geofence ids it is currently in
	Events   []Event
	Trail    map[string][]Location // per-vehicle movement history (memory/snapshot mode only; PostgreSQL stores it in `locations`)
}

const (
	maxEvents = 10000
	maxTrail  = 500 // points kept per vehicle in memory mode
)

// limits bound memory and fan-out so one client cannot exhaust the server.
type limits struct{ fences, vehicles, streams, postgisAbove int }

type App struct {
	mu         sync.RWMutex
	state      State
	hub        *Hub
	file       string // optional JSON snapshot path
	saveTimer  *time.Timer
	apiKey     string   // when set, every request except /health needs it
	limiter    *limiter // per-client rate limit; nil disables
	trustProxy bool     // key rate limits on the proxy-appended X-Forwarded-For entry
	limits     limits
	db         *pgStore // optional PostgreSQL persistence (DATABASE_URL)
	driving    *DrivingStore
}

var idSeq atomic.Uint64

func newID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), idSeq.Add(1))
}

func newApp(file string) *App {
	a := &App{hub: newHub(), file: file, limits: limits{fences: 5000, vehicles: 10000, streams: 500, postgisAbove: 200}, state: State{
		Fences: map[string]Geofence{}, Vehicles: map[string]Vehicle{},
		Rules: map[string]Rule{}, Inside: map[string]map[string]bool{},
	}, driving: newDrivingStore()}
	if data, err := os.ReadFile(file); err == nil {
		json.Unmarshal(data, &a.state)
	}
	return a
}

// save schedules a debounced snapshot (at most one write per saveDelay); call with a.mu held.
func (a *App) save() {
	if a.file == "" || a.saveTimer != nil {
		return
	}
	a.saveTimer = time.AfterFunc(saveDelay, a.Flush)
}

const saveDelay = 250 * time.Millisecond

// Flush writes the snapshot now (atomically via rename). Called by the debounce timer and on shutdown.
func (a *App) Flush() {
	if a.file == "" {
		return
	}
	a.mu.Lock()
	if a.saveTimer != nil {
		a.saveTimer.Stop()
		a.saveTimer = nil
	}
	data, err := json.Marshal(a.state)
	a.mu.Unlock()
	if err == nil {
		tmp := a.file + ".tmp"
		if err = os.WriteFile(tmp, data, 0o644); err == nil {
			err = os.Rename(tmp, a.file)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "persist failed:", err)
	}
}

// Hub fans events out to SSE clients without ever blocking the publisher.
type Hub struct {
	mu      sync.Mutex
	clients map[chan Event]struct{}
}

func newHub() *Hub { return &Hub{clients: map[chan Event]struct{}{}} }

func (h *Hub) subscribe() chan Event { return h.subscribeMax(0) }

// subscribeMax registers a client, or returns nil when max (>0) clients are already connected.
func (h *Hub) subscribeMax(max int) chan Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	if max > 0 && len(h.clients) >= max {
		return nil
	}
	ch := make(chan Event, 32)
	h.clients[ch] = struct{}{}
	return ch
}

func (h *Hub) unsubscribe(ch chan Event) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *Hub) broadcast(events []Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range events {
		for ch := range h.clients {
			select {
			case ch <- e:
			default: // slow client: drop; history still has the event
			}
		}
	}
}

// useDB switches persistence to PostgreSQL: loads existing rows, starts listening for other instances' changes,
// then writes every local change through the queue.
func (a *App) useDB(ctx context.Context, url string) error {
	db, err := openPG(ctx, url)
	if err != nil {
		return err
	}
	st, err := db.load(ctx)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.state, a.db = st, db
	a.mu.Unlock()
	lctx, cancel := context.WithCancel(context.Background())
	db.cancel, db.listenDone = cancel, make(chan struct{})
	go func() {
		defer close(db.listenDone)
		db.listen(lctx, a.applyRemote, a.resync)
	}()
	return nil
}

// record queues a durable change; call with a.mu held so database order matches memory order.
func (a *App) record(c Change) {
	if a.db != nil {
		a.db.enqueue(c)
	}
}

// applyRemote pulls one entity another instance changed into memory (and delivers its alert to local clients).
func (a *App) applyRemote(kind, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	switch kind {
	case "fence":
		if f, err := scanFence(a.db.pool.QueryRow(ctx, `SELECT `+fenceCols+` FROM geofences WHERE id=$1`, id)); err == nil {
			a.mu.Lock()
			a.state.Fences[f.ID] = f
			a.mu.Unlock()
		}
	case "vehicle":
		if v, err := scanVehicle(a.db.pool.QueryRow(ctx, `SELECT `+vehicleCols+` FROM vehicles WHERE id=$1`, id)); err == nil {
			a.mu.Lock()
			a.state.Vehicles[v.ID] = v
			a.state.Inside[v.ID] = insideFences(a.state.Fences, v.Location)
			a.mu.Unlock()
		}
	case "rule":
		r, err := scanRule(a.db.pool.QueryRow(ctx, `SELECT `+ruleCols+` FROM alert_rules WHERE id=$1`, id))
		a.mu.Lock()
		if err == nil {
			a.state.Rules[r.ID] = r
		} else if errors.Is(err, pgx.ErrNoRows) { // deleted on another instance
			delete(a.state.Rules, id)
		}
		a.mu.Unlock()
	case "event":
		if e, err := scanEvent(a.db.pool.QueryRow(ctx, `SELECT `+eventCols+` FROM violations WHERE event_id=$1`, id)); err == nil {
			a.mu.Lock()
			a.state.Events = append(a.state.Events, e)
			if over := len(a.state.Events) - maxEvents; over > 0 {
				a.state.Events = a.state.Events[over:]
			}
			a.mu.Unlock()
			a.hub.broadcast([]Event{e})
		}
	}
}

// resync reloads everything after the notification connection dropped, since notifications may have been missed.
func (a *App) resync() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if st, err := a.db.load(ctx); err == nil {
		a.mu.Lock()
		st.Events = a.state.Events
		a.state = st
		a.mu.Unlock()
	}
}

// insideSet returns the ids of fences containing the point. With many fences and PostGIS available it asks
// the database (GiST index) instead of scanning every polygon in memory.
func (a *App) insideSet(ctx context.Context, lat, lon float64) map[string]bool {
	a.mu.RLock()
	many := len(a.state.Fences) > a.limits.postgisAbove
	a.mu.RUnlock()
	if db := a.db; db != nil && db.postgis && many {
		db.sync() // fences created a moment ago must be visible to the query
		if ids, err := db.containing(ctx, lat, lon); err == nil {
			out := make(map[string]bool, len(ids))
			for _, id := range ids {
				out[id] = true
			}
			return out
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return insideFences(a.state.Fences, &Location{Latitude: lat, Longitude: lon})
}

// appendTrail records a location in memory mode; with a database the `locations` table is the source of truth.
// Call with a.mu held.
func (a *App) appendTrail(vehicle string, loc Location) {
	if a.db != nil {
		return
	}
	if a.state.Trail == nil {
		a.state.Trail = map[string][]Location{}
	}
	t := append(a.state.Trail[vehicle], loc)
	if over := len(t) - maxTrail; over > 0 {
		t = t[over:]
	}
	a.state.Trail[vehicle] = t
}
