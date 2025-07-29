package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type TracerMetrics struct {
	Success       *prometheus.CounterVec
	DurationQuery *prometheus.HistogramVec
	// ManagerStats  *prometheus.GaugeVec
	// ErrCounter    *prometheus.CounterVec
}

func NewTracerMetrics() *TracerMetrics {
	return &TracerMetrics{
		Success: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "MD_Success_add",
		}, []string{"type"}),

		DurationQuery: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "MD_Duration_parse_query",
			Buckets: prometheus.DefBuckets,
		}, []string{"type"}),

		// ManagerStats: prometheus.NewGaugeVec(
		// 	prometheus.GaugeOpts{
		// 		Name: "MD_Manager_parser_stats",
		// 	},
		// 	[]string{"stats"},
		// ),
	}
}
