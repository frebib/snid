package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "snid"
)

type ServerCollector struct {
	connCount  *prometheus.CounterVec
	connErrors *prometheus.CounterVec
	inflight   *prometheus.GaugeVec
	setupTime  prometheus.Histogram
}

func NewServerCollector() ServerCollector {
	return ServerCollector{
		connCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connections_total",
			Help:      "Total number of connections",
		}, []string{"listener"}),
		connErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connections_errors_total",
			Help:      "Total number of connection errors",
		}, []string{"listener", "error"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "connections_inflight",
			Help:      "Total number of connections inflight now",
		}, []string{"listener"}),
		setupTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "backend_dial_time_seconds",
			Help:      "Time taken to resolve and dial the connection to the backend",
		}),
	}
}

func (c *ServerCollector) Describe(ch chan<- *prometheus.Desc) {
	c.connCount.Describe(ch)
	c.connErrors.Describe(ch)
	c.inflight.Describe(ch)
	c.setupTime.Describe(ch)
}

func (c *ServerCollector) Collect(ch chan<- prometheus.Metric) {
	c.connCount.Collect(ch)
	c.connErrors.Collect(ch)
	c.inflight.Collect(ch)
	c.setupTime.Collect(ch)
}
