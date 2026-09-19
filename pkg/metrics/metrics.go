package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Info        *prometheus.GaugeVec
	ReqDuration *prometheus.HistogramVec
	ReqCounter  *prometheus.CounterVec
}

func Init(version, env string) *Metrics {
	m := &Metrics{
		Info: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "app",
				Name:      "info",
				Help:      "Application Information",
			}, []string{"version", "environment"},
		),
		ReqDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "app",
				Name:      "request_duration",
				Help:      "Request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			}, []string{"path", "method", "status_code"},
		),
		ReqCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "app",
				Name:      "request_total",
				Help:      "Total number of requests",
			}, []string{"path", "method", "status_code"},
		),
	}
	m.Info.WithLabelValues(version, env).Set(1)

	prometheus.MustRegister(m.ReqDuration, m.Info, m.ReqCounter)
	return m
}

func (m *Metrics) ExporterHandler() http.Handler {
	return promhttp.Handler()
}
