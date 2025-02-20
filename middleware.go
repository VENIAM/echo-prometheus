// Package echoprometheus implements prometheus metrics middleware
package echoprometheus

import (
	"reflect"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Config responsible to configure middleware
type Config struct {
	HandlerLabelMappingFunc func(c echo.Context) string
	Skipper                 middleware.Skipper
	Namespace               string
	Subsystem               string
	Buckets                 []float64
	NormalizeHTTPStatus     bool
}

// DefaultHandlerLabelMappingFunc returns the handler path
func DefaultHandlerLabelMappingFunc(c echo.Context) string {
	return c.Path()
}

// DefaultSkipper doesn't skip anything
func DefaultSkipper(c echo.Context) bool {
	return false
}

const (
	httpRequestsCount    = "requests_total"
	httpRequestsDuration = "request_duration_seconds"
	notFoundPath         = "/not-found"
)

// DefaultConfig has the default instrumentation config
var DefaultConfig = Config{
	Namespace: "echo",
	Subsystem: "http",
	Buckets: []float64{
		0.0005,
		0.001, // 1ms
		0.002,
		0.005,
		0.01, // 10ms
		0.02,
		0.05,
		0.1, // 100 ms
		0.2,
		0.5,
		1.0, // 1s
		2.0,
		5.0,
		10.0, // 10s
		15.0,
		20.0,
		30.0,
	},
	NormalizeHTTPStatus:     true,
	Skipper:                 DefaultSkipper,
	HandlerLabelMappingFunc: DefaultHandlerLabelMappingFunc,
}

// nolint: gomnd
func normalizeHTTPStatus(status int) string {
	if status < 200 {
		return "1xx"
	} else if status < 300 {
		return "2xx"
	} else if status < 400 {
		return "3xx"
	} else if status < 500 {
		return "4xx"
	}
	return "5xx"
}

func isNotFoundHandler(handler echo.HandlerFunc) bool {
	return reflect.ValueOf(handler).Pointer() == reflect.ValueOf(echo.NotFoundHandler).Pointer()
}

// NewConfig returns a new config with default values
func NewConfig() Config {
	return DefaultConfig
}

// MetricsMiddleware returns an echo middleware with default config for instrumentation.
func MetricsMiddleware() echo.MiddlewareFunc {
	return MetricsMiddlewareWithConfig(DefaultConfig)
}

// MetricsMiddlewareWithConfig returns an echo middleware for instrumentation.
func MetricsMiddlewareWithConfig(config Config) echo.MiddlewareFunc {
	httpRequests := promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: config.Namespace,
		Subsystem: config.Subsystem,
		Name:      httpRequestsCount,
		Help:      "Number of HTTP operations",
	}, []string{"status", "method", "handler"})

	httpDuration := promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: config.Namespace,
		Subsystem: config.Subsystem,
		Name:      httpRequestsDuration,
		Help:      "Spend time by processing a route",
		Buckets:   config.Buckets,
	}, []string{"method", "handler"})

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			path := config.HandlerLabelMappingFunc(c)

			// to avoid attack high cardinality of 404
			if isNotFoundHandler(c.Handler()) {
				path = notFoundPath
			}

			begin := time.Now()
			err := next(c)
			dur := time.Since(begin)

			if err != nil {
				c.Error(err)
			}

			if config.Skipper(c) {
				return nil
			}

			httpDuration.WithLabelValues(req.Method, path).Observe(dur.Seconds())

			status := ""
			if config.NormalizeHTTPStatus {
				status = normalizeHTTPStatus(c.Response().Status)
			} else {
				status = strconv.Itoa(c.Response().Status)
			}

			httpRequests.WithLabelValues(status, req.Method, path).Inc()

			return err
		}
	}
}
