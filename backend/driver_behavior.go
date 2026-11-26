package main

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
)

// TelemetryPoint is a single raw reading from a vehicle's telematics unit.
// Distinct from Location (which only carries lat/lon for geofencing): this
// carries speed/heading so we can score driving behavior in real time.
type TelemetryPoint struct {
	VehicleID string    `json:"vehicle_id"`
	Speed     float64   `json:"speed_kmh"`
	Heading   float64   `json:"heading_deg"`
	Latitude  float64   `json:"latitude,omitempty"`
	Longitude float64   `json:"longitude,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// DrivingEvent is a detected harsh-driving incident.
type DrivingEvent struct {
	ID        string    `json:"id"`
	VehicleID string    `json:"vehicle_id"`
	Type      string    `json:"event_type"` // harsh_braking | harsh_acceleration | overspeed | sharp_turn
	Severity  string    `json:"severity"`   // low | medium | high
	Speed     float64   `json:"speed_kmh"`
	Detail    string    `json:"detail"`
	Timestamp time.Time `json:"timestamp"`
}

// DriverScore is the rolling behavior score for one vehicle/driver.
type DriverScore struct {
	VehicleID         string    `json:"vehicle_id"`
	Score             int       `json:"score"` // 0-100, 100 = flawless
	HarshBraking      int       `json:"harsh_braking_count"`
	HarshAcceleration int       `json:"harsh_acceleration_count"`
	Overspeed         int       `json:"overspeed_count"`
	SharpTurn         int       `json:"sharp_turn_count"`
	LastUpdated       time.Time `json:"last_updated"`
}

type vehicleTelemetryState struct {
	lastSpeed   float64
	lastHeading float64
	lastTS      time.Time
	hasPrev     bool
}

// DrivingStore holds in-memory driver-behavior state. Kept separate from the
// Postgres-backed State/pgStore so this feature works standalone (no schema
// migration needed) and can be swapped for durable storage later.
type DrivingStore struct {
	mu          sync.RWMutex
	last        map[string]vehicleTelemetryState
	scores      map[string]*DriverScore
	events      map[string][]DrivingEvent // vehicleID -> events, newest first
	speedLimit  float64                   // km/h, default threshold for overspeed
	harshBrake  float64                   // m/s^2, negative accel beyond this is harsh braking
	harshAccel  float64                   // m/s^2, positive accel beyond this is harsh acceleration
	turnLimitDg float64                   // degrees of heading change within a short window
}

func newDrivingStore() *DrivingStore {
	return &DrivingStore{
		last:        map[string]vehicleTelemetryState{},
		scores:      map[string]*DriverScore{},
		events:      map[string][]DrivingEvent{},
		speedLimit:  envFloat("SPEED_LIMIT_KMH", 80),
		harshBrake:  envFloat("HARSH_BRAKE_MS2", 3.5),
		harshAccel:  envFloat("HARSH_ACCEL_MS2", 3.0),
		turnLimitDg: envFloat("SHARP_TURN_DEG", 45),
	}
}

func headingDelta(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	if d > 180 {
		d = 360 - d
	}
	return d
}

// ingest applies one telemetry point, returns any newly-fired events.
func (d *DrivingStore) ingest(p TelemetryPoint) []DrivingEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	var fired []DrivingEvent
	prev, ok := d.last[p.VehicleID]

	if ok && p.Timestamp.After(prev.lastTS) {
		dt := p.Timestamp.Sub(prev.lastTS).Seconds()
		if dt > 0 && dt < 30 { // ignore stale/out-of-order gaps
			accel := ((p.Speed - prev.lastSpeed) * 1000 / 3600) / dt // km/h -> m/s, over seconds
			switch {
			case accel <= -d.harshBrake:
				fired = append(fired, d.record(p, "harsh_braking", severityFor(-accel, d.harshBrake), "deceleration "+trimF(-accel)+" m/s2"))
			case accel >= d.harshAccel:
				fired = append(fired, d.record(p, "harsh_acceleration", severityFor(accel, d.harshAccel), "acceleration "+trimF(accel)+" m/s2"))
			}

			if dt < 10 {
				delta := headingDelta(p.Heading, prev.lastHeading)
				if delta >= d.turnLimitDg && p.Speed > 15 {
					fired = append(fired, d.record(p, "sharp_turn", severityFor(delta, d.turnLimitDg), trimF(delta)+" deg heading change at "+trimF(p.Speed)+" km/h"))
				}
			}
		}
	}

	if p.Speed > d.speedLimit {
		fired = append(fired, d.record(p, "overspeed", severityFor(p.Speed-d.speedLimit, 20), trimF(p.Speed)+" km/h in a "+trimF(d.speedLimit)+" km/h zone"))
	}

	d.last[p.VehicleID] = vehicleTelemetryState{lastSpeed: p.Speed, lastHeading: p.Heading, lastTS: p.Timestamp, hasPrev: true}
	d.recomputeScore(p.VehicleID)
	return fired
}

func severityFor(magnitude, threshold float64) string {
	switch {
	case magnitude >= threshold*2:
		return "high"
	case magnitude >= threshold*1.3:
		return "medium"
	default:
		return "low"
	}
}

func trimF(f float64) string {
	return fmt.Sprintf("%.1f", f)
}

// record appends an event under the caller's lock and returns it.
func (d *DrivingStore) record(p TelemetryPoint, typ, severity, detail string) DrivingEvent {
	ev := DrivingEvent{
		ID: newID("drv"), VehicleID: p.VehicleID, Type: typ, Severity: severity,
		Speed: p.Speed, Detail: detail, Timestamp: p.Timestamp,
	}
	d.events[p.VehicleID] = append([]DrivingEvent{ev}, d.events[p.VehicleID]...)
	if len(d.events[p.VehicleID]) > 500 {
		d.events[p.VehicleID] = d.events[p.VehicleID][:500]
	}
	return ev
}

// recomputeScore must be called with d.mu held.
func (d *DrivingStore) recomputeScore(vehicleID string) {
	var hb, ha, os_, st int
	for _, e := range d.events[vehicleID] {
		switch e.Type {
		case "harsh_braking":
			hb++
		case "harsh_acceleration":
			ha++
		case "overspeed":
			os_++
		case "sharp_turn":
			st++
		}
	}
	score := 100 - (hb*3 + ha*3 + os_*2 + st*2)
	if score < 0 {
		score = 0
	}
	d.scores[vehicleID] = &DriverScore{
		VehicleID: vehicleID, Score: score,
		HarshBraking: hb, HarshAcceleration: ha, Overspeed: os_, SharpTurn: st,
		LastUpdated: time.Now(),
	}
}

func (d *DrivingStore) score(vehicleID string) (DriverScore, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	s, ok := d.scores[vehicleID]
	if !ok {
		return DriverScore{}, false
	}
	return *s, true
}

func (d *DrivingStore) allScores() []DriverScore {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]DriverScore, 0, len(d.scores))
	for _, s := range d.scores {
		out = append(out, *s)
	}
	return out
}

func (d *DrivingStore) history(vehicleID string) []DrivingEvent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]DrivingEvent(nil), d.events[vehicleID]...)
}

// --- HTTP handlers ---

func (a *App) ingestTelemetry(w http.ResponseWriter, r *http.Request) {
	var p TelemetryPoint
	if !decode(w, r, &p) {
		return
	}
	if p.VehicleID == "" {
		fail(w, r, 400, "vehicle_id is required")
		return
	}
	if p.Timestamp.IsZero() {
		p.Timestamp = time.Now().UTC()
	}
	fired := a.driving.ingest(p)
	score, _ := a.driving.score(p.VehicleID)
	reply(w, r, 200, map[string]any{"events": fired, "score": score})
}

func (a *App) driverScore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s, ok := a.driving.score(id)
	if !ok {
		fail(w, r, 404, "no telemetry yet for this vehicle")
		return
	}
	reply(w, r, 200, s)
}

func (a *App) driverScores(w http.ResponseWriter, r *http.Request) {
	scores := a.driving.allScores()
	page, total := paginate(scores, r.URL.Query())
	reply(w, r, 200, map[string]any{"scores": page, "total": total})
}

func (a *App) driverEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	events := a.driving.history(id)
	page, total := paginate(events, r.URL.Query())
	reply(w, r, 200, map[string]any{"events": page, "total": total})
}
