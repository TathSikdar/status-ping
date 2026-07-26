package monitor

import "time"

// Endpoint is one target to poll. ExpectStatus/ExpectBody are optional
// assertions: a 200 with the wrong body is still an outage.
type Endpoint struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	ExpectStatus int    `json:"expect_status,omitempty"` // exact code; 0 means "any 2xx/3xx"
	ExpectBody   string `json:"expect_body,omitempty"`   // required substring in the response body
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
