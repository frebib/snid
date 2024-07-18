package main

import (
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "snid"
)

type ServerCollector struct {
	connCount        *prometheus.CounterVec
	connErrors       *prometheus.CounterVec
	inflight         *prometheus.GaugeVec
	setupTime        prometheus.Histogram
	clientReadBytes  prometheus.Counter
	clientWriteBytes prometheus.Counter
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
		clientReadBytes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "client_read_bytes_total",
			Help:      "Total number of bytes read from clients",
		}),
		clientWriteBytes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "client_write_bytes_total",
			Help:      "Total number of bytes written to clients",
		}),
	}
}

func (c *ServerCollector) Describe(ch chan<- *prometheus.Desc) {
	c.connCount.Describe(ch)
	c.connErrors.Describe(ch)
	c.inflight.Describe(ch)
	c.setupTime.Describe(ch)
	c.clientReadBytes.Describe(ch)
	c.clientWriteBytes.Describe(ch)
}

func (c *ServerCollector) Collect(ch chan<- prometheus.Metric) {
	c.connCount.Collect(ch)
	c.connErrors.Collect(ch)
	c.inflight.Collect(ch)
	c.setupTime.Collect(ch)
	c.clientReadBytes.Collect(ch)
	c.clientWriteBytes.Collect(ch)
}

func InstrumentedConn(conn net.Conn, readCount, writeCount prometheus.Counter) net.Conn {
	return &instrumentedConn{conn, readCount, writeCount}
}

type instrumentedConn struct {
	inner       net.Conn
	read, write prometheus.Counter
}

func (i instrumentedConn) Read(b []byte) (int, error) {
	n, err := i.inner.Read(b)
	i.read.Add(float64(n))
	return n, err
}

func (i instrumentedConn) Write(b []byte) (int, error) {
	n, err := i.inner.Write(b)
	i.write.Add(float64(n))
	return n, err
}

func (i instrumentedConn) Close() error {
	return i.inner.Close()
}

func (i instrumentedConn) LocalAddr() net.Addr {
	return i.inner.LocalAddr()
}

func (i instrumentedConn) RemoteAddr() net.Addr {
	return i.inner.RemoteAddr()
}

func (i instrumentedConn) SetDeadline(t time.Time) error {
	return i.inner.SetDeadline(t)
}

func (i instrumentedConn) SetReadDeadline(t time.Time) error {
	return i.inner.SetReadDeadline(t)
}

func (i instrumentedConn) SetWriteDeadline(t time.Time) error {
	return i.inner.SetWriteDeadline(t)
}

var _ net.Conn = &instrumentedConn{}
