package monitor

import "time"

// Endpoint is one target to poll.
type Endpoint struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Status is the result of a single probe. Shared by WS, Redis, and Mongo.
type Status struct {
	Name      string    `json:"name" bson:"name"`
	URL       string    `json:"url" bson:"url"`
	Up        bool      `json:"up" bson:"up"`
	LatencyMs int64     `json:"latency_ms" bson:"latency_ms"`
	Code      int       `json:"code" bson:"code"`
	Error     string    `json:"error,omitempty" bson:"error,omitempty"`
	Ts        time.Time `json:"ts" bson:"ts"`
}
