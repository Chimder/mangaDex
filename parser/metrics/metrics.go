package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type TracerMetrics struct {
	ErrCounter    *prometheus.CounterVec
	ManagerStats  *prometheus.GaugeVec
	Success       *prometheus.CounterVec
	DurationQuery *prometheus.HistogramVec
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

		ManagerStats: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "MD_Manager_parser_stats",
			},
			[]string{"stats"},
		),

		ErrCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "MD_Error",
			},
			[]string{"err"},
		),
	}
}
