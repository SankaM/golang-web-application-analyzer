package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed.",
		},
		[]string{"method", "path", "status"},
	)

	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	AnalyzedURLsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "analyzed_urls_total",
			Help: "Total number of URLs successfully analyzed.",
		},
	)
)

func init() {
	prometheus.MustRegister(RequestsTotal, RequestDuration, AnalyzedURLsTotal)
}
