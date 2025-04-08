package main

import (
	"net"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "snid"
)

type ServerCollector struct {
	connCount  *prometheus.CounterVec
	connErrors *prometheus.CounterVec
	inflight   *prometheus.GaugeVec

	beConnCount  *prometheus.CounterVec
	beSetupTime  *prometheus.HistogramVec
	beWriteBytes *prometheus.CounterVec
	beReadBytes  *prometheus.CounterVec
}

func NewServerCollector() ServerCollector {
	return ServerCollector{
		connCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connections_total",
			Help:      "Total number of connections",
		}, []string{"listener"}),
		inflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "connections_inflight",
			Help:      "Total number of connections inflight now",
		}, []string{"listener"}),
		connErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connection_errors_total",
			Help:      "Total number of connection errors",
		}, []string{"listener", "backend", "cause", "error"}),

		beConnCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "backend",
			Name:      "connections_total",
			Help:      "Total number of connections",
		}, []string{"listener", "backend"}),
		beSetupTime: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "backend",
			Name:      "dial_time_seconds",
			Help:      "Time taken to resolve and dial the connection to the backend",
		}, []string{"listener", "backend"}),
		beWriteBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "backend",
			Name:      "read_bytes_total",
			Help:      "Total number of bytes read from clients and written to the backend",
		}, []string{"listener", "backend"}),
		beReadBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "backend",
			Name:      "write_bytes_total",
			Help:      "Total number of bytes read from the backend and written to clients",
		}, []string{"listener", "backend"}),
	}
}

func (c *ServerCollector) Describe(ch chan<- *prometheus.Desc) {
	c.connCount.Describe(ch)
	c.connErrors.Describe(ch)
	c.inflight.Describe(ch)

	c.beConnCount.Describe(ch)
	c.beSetupTime.Describe(ch)
	c.beWriteBytes.Describe(ch)
	c.beReadBytes.Describe(ch)
}

func (c *ServerCollector) Collect(ch chan<- prometheus.Metric) {
	c.connCount.Collect(ch)
	c.connErrors.Collect(ch)
	c.inflight.Collect(ch)

	c.beConnCount.Collect(ch)
	c.beSetupTime.Collect(ch)
	c.beWriteBytes.Collect(ch)
	c.beReadBytes.Collect(ch)
}

func InstrumentedConn(conn net.Conn, readCount, writeCount prometheus.Counter) net.Conn {
	return &instrumentedConn{conn, readCount, writeCount}
}

type instrumentedConn struct {
	net.Conn
	read, write prometheus.Counter
}

func (i instrumentedConn) Read(b []byte) (int, error) {
	n, err := i.Conn.Read(b)
	i.read.Add(float64(n))
	return n, err
}

func (i instrumentedConn) Write(b []byte) (int, error) {
	n, err := i.Conn.Write(b)
	i.write.Add(float64(n))
	return n, err
}

var _ net.Conn = &instrumentedConn{}
