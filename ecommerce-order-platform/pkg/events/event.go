package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const CurrentVersion = 1

type Event struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	OrderID       string          `json:"order_id"`
	CorrelationID string          `json:"correlation_id"`
	CausationID   *string         `json:"causation_id"`
	Source        string          `json:"source"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Version       int             `json:"version"`
	Payload       json.RawMessage `json:"payload"`
}

type NewEventParams struct {
	EventType     string
	OrderID       string
	CorrelationID string
	CausationID   *string
	Source        string
	Payload       any
}

func New(params NewEventParams) (Event, error) {
	payload, err := json.Marshal(params.Payload)
	if err != nil {
		return Event{}, err
	}

	correlationID := params.CorrelationID
	if correlationID == "" {
		correlationID = uuid.NewString()
	}

	return Event{
		EventID:       uuid.NewString(),
		EventType:     params.EventType,
		OrderID:       params.OrderID,
		CorrelationID: correlationID,
		CausationID:   params.CausationID,
		Source:        params.Source,
		OccurredAt:    time.Now().UTC(),
		Version:       CurrentVersion,
		Payload:       payload,
	}, nil
}

func Marshal(event Event) ([]byte, error) {
	return json.Marshal(event)
}

func Unmarshal(data []byte) (Event, error) {
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func DecodePayload[T any](event Event) (T, error) {
	var payload T
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return payload, err
	}
	return payload, nil
}
