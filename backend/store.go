package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schema = `
CREATE TABLE IF NOT EXISTS geofences (
  id text PRIMARY KEY, name text NOT NULL, description text NOT NULL DEFAULT '', category text NOT NULL,
  coordinates jsonb NOT NULL, created_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS vehicles (
  id text PRIMARY KEY, vehicle_number text NOT NULL, driver_name text NOT NULL, vehicle_type text NOT NULL, phone text NOT NULL,
  status text NOT NULL, created_at timestamptz NOT NULL, latitude double precision, longitude double precision, located_at timestamptz);
CREATE UNIQUE INDEX IF NOT EXISTS vehicles_number_ci ON vehicles (lower(vehicle_number));
CREATE TABLE IF NOT EXISTS alert_rules (
  id text PRIMARY KEY, geofence_id text NOT NULL REFERENCES geofences(id), vehicle_id text REFERENCES vehicles(id),
  event_type text NOT NULL, status text NOT NULL, created_at timestamptz NOT NULL);
CREATE INDEX IF NOT EXISTS alert_rules_fence ON alert_rules (geofence_id);
CREATE TABLE IF NOT EXISTS violations (
  id text PRIMARY KEY, event_id text NOT NULL UNIQUE, event_type text NOT NULL, vehicle_id text NOT NULL, vehicle_number text NOT NULL,
  driver_name text NOT NULL, geofence_id text NOT NULL, geofence_name text NOT NULL, category text NOT NULL,
  latitude double precision NOT NULL, longitude double precision NOT NULL, ts timestamptz NOT NULL);
CREATE INDEX IF NOT EXISTS violations_ts ON violations (ts DESC);
CREATE INDEX IF NOT EXISTS violations_vehicle_ts ON violations (vehicle_id, ts DESC);
CREATE TABLE IF NOT EXISTS locations (
  id bigserial PRIMARY KEY, vehicle_id text NOT NULL REFERENCES vehicles(id),
  latitude double precision NOT NULL, longitude double precision NOT NULL, ts timestamptz NOT NULL);
CREATE INDEX IF NOT EXISTS locations_vehicle_ts ON locations (vehicle_id, ts DESC);
CREATE INDEX IF NOT EXISTS violations_fence_ts ON violations (geofence_id, ts DESC);`

// Change is one durable mutation, applied in order by the writer goroutine.
type Change struct {
	Fence      *Geofence
	Vehicle    *Vehicle
	Rule       *Rule
	DeleteRule string // alert rule id to remove
	Trail      bool   // also append Vehicle.Location to the movement history
	Events     []Event
	done       chan struct{} // barrier: closed once everything queued before it is written
}

// geomExpr builds a PostGIS polygon from a JSON array of [lat, lon] pairs (PostGIS wants x=lon, y=lat).
func geomExpr(jsonCoords string) string {
	return `ST_SetSRID(ST_MakePolygon(ST_MakeLine(ARRAY(SELECT ST_MakePoint((p->>1)::float8,(p->>0)::float8) FROM jsonb_array_elements(` +
		jsonCoords + `) WITH ORDINALITY t(p,i) ORDER BY i))),4326)`
}

const postgisSchema = `
ALTER TABLE geofences ADD COLUMN IF NOT EXISTS geom geometry(Polygon,4326);
UPDATE geofences SET geom = ` + `ST_SetSRID(ST_MakePolygon(ST_MakeLine(ARRAY(SELECT ST_MakePoint((p->>1)::float8,(p->>0)::float8) FROM jsonb_array_elements(coordinates) WITH ORDINALITY t(p,i) ORDER BY i))),4326)` + ` WHERE geom IS NULL;
CREATE INDEX IF NOT EXISTS geofences_geom ON geofences USING gist (geom);`

// pgStore persists to PostgreSQL. Reads are served from memory; history and (for large fence counts)
// containment come from the database. Instances coordinate through LISTEN/NOTIFY on `dhruva`.
type pgStore struct {
	pool       *pgxpool.Pool
	q          chan Change
	quit       chan struct{}
	postgis    bool
	instance   string // identifies this process so it ignores its own notifications
	cancel     context.CancelFunc
	listenDone chan struct{}
}

const channel = "dhruva"

func openPG(ctx context.Context, url string) (*pgStore, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err = pool.Exec(ctx, schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	id := make([]byte, 6)
	rand.Read(id)
	s := &pgStore{pool: pool, q: make(chan Change, 4096), quit: make(chan struct{}), instance: hex.EncodeToString(id)}
	if _, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS postgis`); err != nil {
		log.Printf("PostGIS unavailable (%v); containment stays in memory", err)
	} else if _, err = pool.Exec(ctx, postgisSchema); err != nil {
		log.Printf("PostGIS schema failed (%v); containment stays in memory", err)
	} else {
		s.postgis = true
	}
	go s.writer()
	return s, nil
}

func (s *pgStore) writer() {
	defer close(s.quit)
	for c := range s.q {
		if c.done != nil {
			close(c.done)
			continue
		}
		if err := s.apply(context.Background(), c); err != nil {
			log.Printf("db write failed: %v", err)
		}
	}
}

func (s *pgStore) apply(ctx context.Context, c Change) error {
	b := &pgx.Batch{}
	if f := c.Fence; f != nil {
		coords, _ := json.Marshal(f.Coordinates)
		b.Queue(`INSERT INTO geofences (id,name,description,category,coordinates,created_at) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`, f.ID, f.Name, f.Description, f.Category, coords, f.CreatedAt)
		if s.postgis {
			b.Queue(`UPDATE geofences SET geom = `+geomExpr("coordinates")+` WHERE id=$1`, f.ID)
		}
		s.notify(b, "fence", f.ID)
	}
	if v := c.Vehicle; v != nil {
		var lat, lon *float64
		var at *time.Time
		if v.Location != nil {
			lat, lon, at = &v.Location.Latitude, &v.Location.Longitude, &v.Location.Timestamp
		}
		b.Queue(`INSERT INTO vehicles (id,vehicle_number,driver_name,vehicle_type,phone,status,created_at,latitude,longitude,located_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (id) DO UPDATE SET latitude=EXCLUDED.latitude, longitude=EXCLUDED.longitude, located_at=EXCLUDED.located_at`,
			v.ID, v.Number, v.Driver, v.Type, v.Phone, v.Status, v.CreatedAt, lat, lon, at)
		if c.Trail && v.Location != nil {
			b.Queue(`INSERT INTO locations (vehicle_id,latitude,longitude,ts) VALUES ($1,$2,$3,$4)`, v.ID, v.Location.Latitude, v.Location.Longitude, v.Location.Timestamp)
		}
		s.notify(b, "vehicle", v.ID)
	}
	if r := c.Rule; r != nil {
		b.Queue(`INSERT INTO alert_rules VALUES ($1,$2,NULLIF($3,''),$4,$5,$6) ON CONFLICT (id) DO NOTHING`, r.ID, r.FenceID, r.VehicleID, r.EventType, r.Status, r.CreatedAt)
		s.notify(b, "rule", r.ID)
	}
	if id := c.DeleteRule; id != "" {
		b.Queue(`DELETE FROM alert_rules WHERE id=$1`, id)
		s.notify(b, "rule", id)
	}
	for _, e := range c.Events {
		b.Queue(`INSERT INTO violations VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (event_id) DO NOTHING`,
			e.ID, e.EventID, e.EventType, e.VehicleID, e.VehicleNum, e.Driver, e.FenceID, e.FenceName, e.Category, e.Latitude, e.Longitude, e.Timestamp)
		s.notify(b, "event", e.EventID)
	}
	res := s.pool.SendBatch(ctx, b)
	defer res.Close()
	for i := 0; i < b.Len(); i++ {
		if _, err := res.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// notify queues a NOTIFY (delivered on commit, after the row is visible) so other instances pull the change.
func (s *pgStore) notify(b *pgx.Batch, kind, id string) {
	payload, _ := json.Marshal(map[string]string{"o": s.instance, "k": kind, "id": id})
	b.Queue(`SELECT pg_notify($1,$2)`, channel, string(payload))
}

// enqueue applies backpressure (blocks) rather than dropping writes when the queue is full.
func (s *pgStore) enqueue(c Change) { s.q <- c }

// sync blocks until every change queued so far is committed (read-your-writes for history).
func (s *pgStore) sync() {
	done := make(chan struct{})
	s.q <- Change{done: done}
	<-done
}

func (s *pgStore) close() {
	if s.cancel != nil {
		s.cancel()
		<-s.listenDone
	}
	close(s.q)
	<-s.quit
	s.pool.Close()
}

// listen forwards other instances' notifications to handle; after a reconnect it calls resync to catch up on missed ones.
func (s *pgStore) listen(ctx context.Context, handle func(kind, id string), resync func()) {
	for first := true; ctx.Err() == nil; first = false {
		err := func() error {
			conn, err := s.pool.Acquire(ctx)
			if err != nil {
				return err
			}
			defer conn.Release()
			if _, err = conn.Exec(ctx, "LISTEN "+channel); err != nil {
				return err
			}
			if !first {
				resync()
			}
			for {
				n, err := conn.Conn().WaitForNotification(ctx)
				if err != nil {
					return err
				}
				var m struct{ O, K, ID string }
				if json.Unmarshal([]byte(n.Payload), &m) == nil && m.O != s.instance {
					handle(m.K, m.ID)
				}
			}
		}()
		if ctx.Err() != nil {
			return
		}
		log.Printf("listener: %v; reconnecting", err)
		time.Sleep(time.Second)
	}
}

// containing returns the ids of geofences covering the point, using the GiST index.
func (s *pgStore) containing(ctx context.Context, lat, lon float64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM geofences WHERE geom IS NOT NULL AND ST_Covers(geom, ST_SetSRID(ST_MakePoint($2,$1),4326))`, lat, lon)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type scanner interface{ Scan(...any) error }

const (
	fenceCols   = `id,name,description,category,coordinates,created_at`
	vehicleCols = `id,vehicle_number,driver_name,vehicle_type,phone,status,created_at,latitude,longitude,located_at`
	ruleCols    = `id,geofence_id,COALESCE(vehicle_id,''),event_type,status,created_at`
	eventCols   = `id,event_id,event_type,vehicle_id,vehicle_number,driver_name,geofence_id,geofence_name,category,latitude,longitude,ts`
)

func scanFence(r scanner) (f Geofence, err error) {
	var coords []byte
	if err = r.Scan(&f.ID, &f.Name, &f.Description, &f.Category, &coords, &f.CreatedAt); err == nil {
		err = json.Unmarshal(coords, &f.Coordinates)
	}
	return
}

func scanVehicle(r scanner) (v Vehicle, err error) {
	var lat, lon *float64
	var at *time.Time
	if err = r.Scan(&v.ID, &v.Number, &v.Driver, &v.Type, &v.Phone, &v.Status, &v.CreatedAt, &lat, &lon, &at); err == nil && lat != nil && lon != nil && at != nil {
		v.Location = &Location{*lat, *lon, *at}
	}
	return
}

func scanRule(r scanner) (x Rule, err error) {
	err = r.Scan(&x.ID, &x.FenceID, &x.VehicleID, &x.EventType, &x.Status, &x.CreatedAt)
	return
}

func scanEvent(r scanner) (e Event, err error) {
	err = r.Scan(&e.ID, &e.EventID, &e.EventType, &e.VehicleID, &e.VehicleNum, &e.Driver, &e.FenceID, &e.FenceName, &e.Category, &e.Latitude, &e.Longitude, &e.Timestamp)
	return
}

// load rebuilds the in-memory state (events are not loaded: history is read from the database).
func (s *pgStore) load(ctx context.Context) (State, error) {
	st := State{Fences: map[string]Geofence{}, Vehicles: map[string]Vehicle{}, Rules: map[string]Rule{}, Inside: map[string]map[string]bool{}}
	rows, err := s.pool.Query(ctx, `SELECT `+fenceCols+` FROM geofences`)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		f, err := scanFence(rows)
		if err != nil {
			rows.Close()
			return st, err
		}
		st.Fences[f.ID] = f
	}
	rows.Close()
	if rows, err = s.pool.Query(ctx, `SELECT `+vehicleCols+` FROM vehicles`); err != nil {
		return st, err
	}
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			rows.Close()
			return st, err
		}
		st.Vehicles[v.ID] = v
		st.Inside[v.ID] = insideFences(st.Fences, v.Location)
	}
	rows.Close()
	if rows, err = s.pool.Query(ctx, `SELECT `+ruleCols+` FROM alert_rules`); err != nil {
		return st, err
	}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			rows.Close()
			return st, err
		}
		st.Rules[r.ID] = r
	}
	rows.Close()
	return st, rows.Err()
}

// insideFences recomputes which fences contain a vehicle's last location.
func insideFences(fences map[string]Geofence, loc *Location) map[string]bool {
	in := map[string]bool{}
	if loc != nil {
		for id, f := range fences {
			if containsPoint(f.Coordinates, loc.Latitude, loc.Longitude) {
				in[id] = true
			}
		}
	}
	return in
}

// history runs the filtered, paginated query on indexed columns. Empty filters are ignored.
func (s *pgStore) history(ctx context.Context, vehicle, fence string, start, end *time.Time, limit, offset int) ([]Event, int, error) {
	const where = `WHERE ($1='' OR vehicle_id=$1) AND ($2='' OR geofence_id=$2) AND ($3::timestamptz IS NULL OR ts>=$3) AND ($4::timestamptz IS NULL OR ts<=$4)`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM violations `+where, vehicle, fence, start, end).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+eventCols+`
		FROM violations `+where+` ORDER BY ts DESC, id DESC LIMIT $5 OFFSET $6`, vehicle, fence, start, end, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// trail returns a vehicle's movement history, newest first, with the matching total for pagination.
func (s *pgStore) trail(ctx context.Context, vehicle string, start, end *time.Time, limit, offset int) ([]Location, int, error) {
	const where = `WHERE vehicle_id=$1 AND ($2::timestamptz IS NULL OR ts>=$2) AND ($3::timestamptz IS NULL OR ts<=$3)`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM locations `+where, vehicle, start, end).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT latitude,longitude,ts FROM locations `+where+` ORDER BY ts DESC, id DESC LIMIT $4 OFFSET $5`, vehicle, start, end, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Location{}
	for rows.Next() {
		var l Location
		if err = rows.Scan(&l.Latitude, &l.Longitude, &l.Timestamp); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}
