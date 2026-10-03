package domain

import (
	"context"
	"time"
)

// ObservationRequest contains only read-only query parameters. Identity and
// data mode are attached by trusted application code, never by the model.
type ObservationRequest struct {
	Capability    string            `json:"capability"`
	Component     string            `json:"component,omitempty"`
	Fields        []string          `json:"fields"`
	Parameters    map[string]string `json:"parameters,omitempty"`
	WindowStart   time.Time         `json:"window_start"`
	WindowEnd     time.Time         `json:"window_end"`
	MaxAgeSeconds int               `json:"max_age_seconds"`
}

type ObservationQuery struct {
	ObservationRequest
	Device DeviceContext `json:"device_context"`
}

type ObservationValue struct {
	Field      string    `json:"field"`
	Value      string    `json:"value"`
	Unit       string    `json:"unit"`
	ObservedAt time.Time `json:"observed_at"`
}

// ObservationData is the monitor wire contract, shared by LIVE and REPLAY.
// Coverage must be explicit even when the provider found no records.
type ObservationData struct {
	Status       string             `json:"status"`
	DataMode     string             `json:"data_mode"`
	DeviceID     string             `json:"device_id"`
	SnapshotID   string             `json:"snapshot_id"`
	MonitoringID string             `json:"monitoring_id"`
	WindowStart  time.Time          `json:"window_start"`
	WindowEnd    time.Time          `json:"window_end"`
	Source       string             `json:"source"`
	RawRef       string             `json:"raw_ref"`
	Values       []ObservationValue `json:"values"`
	Missing      []string           `json:"missing,omitempty"`
	Error        *Failure           `json:"error,omitempty"`
}

type Evidence struct {
	SchemaVersion int              `json:"schema_version"`
	ID            string           `json:"id"`
	Query         ObservationQuery `json:"query"`
	ObservationData
	FetchedAt time.Time `json:"fetched_at"`
}

type Observer interface {
	Observe(context.Context, ObservationQuery) (ObservationData, error)
}

// ObservationSelection selects validated values; arbitrary model prose cannot
// turn a subset of metrics into a whole-device health claim.
type ObservationSelection struct {
	EvidenceID string   `json:"evidence_id"`
	Fields     []string `json:"fields,omitempty"`
}
