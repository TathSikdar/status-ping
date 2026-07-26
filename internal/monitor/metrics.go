package monitor

import "github.com/prometheus/client_golang/prometheus"

var (
	upGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "endpoint_up", Help: "1 if the endpoint's last probe succeeded, else 0",
	}, []string{"endpoint"})

	latencyHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "endpoint_latency_seconds", Help: "Probe latency in seconds",
		Buckets: []float64{.025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"endpoint"})
)

func init() { prometheus.MustRegister(upGauge, latencyHist) }

func recordMetrics(s Status) {
	up := 0.0
	if s.Up {
		up = 1
	}
	upGauge.WithLabelValues(s.Name).Set(up)
	latencyHist.WithLabelValues(s.Name).Observe(float64(s.LatencyMs) / 1000)
}
