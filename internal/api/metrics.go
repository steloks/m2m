package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	registry        *prometheus.Registry
	eventsIngested  prometheus.Counter
	eventsDuplicate prometheus.Counter
	requestDuration *prometheus.HistogramVec
}

func newMetrics() *metrics {
	registry := prometheus.NewRegistry()
	return &metrics{
		registry: registry,
		eventsIngested: register(registry, prometheus.NewCounter(prometheus.CounterOpts{
			Name: "events_ingested_total",
			Help: "New events recorded.",
		})),
		eventsDuplicate: register(registry, prometheus.NewCounter(prometheus.CounterOpts{
			Name: "events_duplicate_total",
			Help: "Events received again with an event_id the device already sent.",
		})),
		requestDuration: register(registry, prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Time to serve HTTP requests, by route and status code.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "status"})),
	}
}

func register[C prometheus.Collector](registry *prometheus.Registry, collector C) C {
	registry.MustRegister(collector)
	return collector
}

func (m *metrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// instrument records how long each request took. Labels only take values the
// service controls, so clients cannot make the number of series grow: the
// route is the matched pattern, such as "GET /devices/{id}", which already
// includes the method, rather than the path or the method the client sent.
func (m *metrics) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)

		route := request.Pattern
		if route == "" {
			route = "unmatched"
		}
		m.requestDuration.
			WithLabelValues(route, strconv.Itoa(recorder.status)).
			Observe(time.Since(start).Seconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}
