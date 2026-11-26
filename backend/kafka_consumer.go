package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// KafkaConfig is read from the environment. Unset KAFKA_BROKERS means the
// consumer is disabled entirely (the REST /vehicles/location endpoint keeps
// working exactly as before) — nothing here is required for local dev or CI.
type KafkaConfig struct {
	Brokers       []string
	Topic         string
	GroupID       string
	Username      string
	Password      string
	SASLMechanism string // "plain" (default when Username set), "scram-sha-256", "scram-sha-512"
	TLS           bool
}

func kafkaConfigFromEnv() (KafkaConfig, bool) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return KafkaConfig{}, false
	}
	cfg := KafkaConfig{
		Brokers:       strings.Split(brokers, ","),
		Topic:         envOr("KAFKA_TOPIC", "vehicle-telemetry"),
		GroupID:       envOr("KAFKA_GROUP_ID", "dhruva-geofencing"),
		Username:      os.Getenv("KAFKA_USERNAME"),
		Password:      os.Getenv("KAFKA_PASSWORD"),
		SASLMechanism: strings.ToLower(envOr("KAFKA_SASL_MECHANISM", "plain")),
	}
	for i, b := range cfg.Brokers {
		cfg.Brokers[i] = strings.TrimSpace(b)
	}
	cfg.TLS = os.Getenv("KAFKA_TLS") == "1" || (os.Getenv("KAFKA_TLS") == "" && cfg.Username != "")
	return cfg, true
}

// telemetryMessage accepts the field-name variants different telemetry
// generators are likely to use (Condense's starter kit included) so the
// consumer doesn't break on a naming mismatch alone.
type telemetryMessage struct {
	VehicleID  string  `json:"vehicle_id"`
	VehicleID2 string  `json:"vehicleId"`
	ID         string  `json:"id"`
	Lat        float64 `json:"latitude"`
	Lat2       float64 `json:"lat"`
	Lon        float64 `json:"longitude"`
	Lon2       float64 `json:"lon"`
	Lon3       float64 `json:"lng"`
	Timestamp  string  `json:"timestamp"`
	Timestamp2 string  `json:"ts"`
	Timestamp3 string  `json:"time"`
	EpochMs    int64   `json:"timestamp_ms"`
}

func (m telemetryMessage) vehicleID() string {
	for _, v := range []string{m.VehicleID, m.VehicleID2, m.ID} {
		if v != "" {
			return v
		}
	}
	return ""
}

func (m telemetryMessage) coords() (lat, lon float64) {
	lat = m.Lat
	if lat == 0 {
		lat = m.Lat2
	}
	lon = m.Lon
	if lon == 0 {
		lon = m.Lon2
	}
	if lon == 0 {
		lon = m.Lon3
	}
	return lat, lon
}

func (m telemetryMessage) when() (time.Time, bool) {
	if m.EpochMs > 0 {
		return time.UnixMilli(m.EpochMs).UTC(), true
	}
	for _, s := range []string{m.Timestamp, m.Timestamp2, m.Timestamp3} {
		if s == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
		if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.UnixMilli(ms).UTC(), true
		}
	}
	return time.Time{}, false
}

// runKafkaConsumer blocks, consuming telemetry until ctx is cancelled. Call it
// in its own goroutine from main. Every message is fed through the exact same
// applyLocationUpdate path the REST endpoint uses, so geofence evaluation,
// alert firing and the live SSE stream behave identically regardless of
// whether a position came in over HTTP or over Condense's Kafka workspace.
func runKafkaConsumer(ctx context.Context, a *App, cfg KafkaConfig) {
	dialer := &kafka.Dialer{Timeout: 10 * time.Second, DualStack: true}
	if cfg.Username != "" {
		mech, err := saslMechanism(cfg)
		if err != nil {
			log.Printf("kafka: %v; consumer disabled", err)
			return
		}
		dialer.SASLMechanism = mech
		dialer.TLS = &tls.Config{}
	} else if cfg.TLS {
		dialer.TLS = &tls.Config{}
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     cfg.Brokers,
		Topic:       cfg.Topic,
		GroupID:     cfg.GroupID,
		Dialer:      dialer,
		MinBytes:    1,
		MaxBytes:    1 << 20,
		StartOffset: kafka.LastOffset,
	})
	defer reader.Close()

	log.Printf("kafka: consuming %q (group %q) from %v", cfg.Topic, cfg.GroupID, cfg.Brokers)
	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("kafka: fetch error: %v (retrying)", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}

		var tm telemetryMessage
		if err := json.Unmarshal(msg.Value, &tm); err != nil {
			log.Printf("kafka: dropping unparseable message at offset %d: %v", msg.Offset, err)
			reader.CommitMessages(ctx, msg)
			continue
		}
		vid := tm.vehicleID()
		lat, lon := tm.coords()
		ts, haveTS := tm.when()
		if !haveTS {
			ts = time.Now().UTC()
		}
		if !validLocationInput(vid, lat, lon, ts) {
			log.Printf("kafka: dropping invalid telemetry at offset %d (vehicle_id=%q lat=%v lon=%v)", msg.Offset, vid, lat, lon)
			reader.CommitMessages(ctx, msg)
			continue
		}

		if _, _, err := a.applyLocationUpdate(ctx, vid, lat, lon, ts, true); err != nil && !errors.Is(err, errStaleTimestamp) {
			log.Printf("kafka: applyLocationUpdate failed for vehicle %q: %v", vid, err)
		}
		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("kafka: commit failed: %v", err)
		}
	}
}

func saslMechanism(cfg KafkaConfig) (sasl.Mechanism, error) {
	switch cfg.SASLMechanism {
	case "", "plain":
		return plain.Mechanism{Username: cfg.Username, Password: cfg.Password}, nil
	case "scram-sha-256":
		return scram.Mechanism(scram.SHA256, cfg.Username, cfg.Password)
	case "scram-sha-512":
		return scram.Mechanism(scram.SHA512, cfg.Username, cfg.Password)
	default:
		return nil, errUnknownSASLMechanism
	}
}

var errUnknownSASLMechanism = errors.New("unknown KAFKA_SASL_MECHANISM (want plain, scram-sha-256 or scram-sha-512)")
