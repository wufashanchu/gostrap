package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 指标收集器
type Metrics struct {
	registry *prometheus.Registry

	// HTTP指标
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpRequestsInFlight prometheus.Gauge

	// gRPC指标
	grpcRequestsTotal   *prometheus.CounterVec
	grpcRequestDuration *prometheus.HistogramVec

	// 业务指标
	businessCounter   *prometheus.CounterVec
	businessGauge     *prometheus.GaugeVec
	businessHistogram *prometheus.HistogramVec
}

// Config 指标配置
type Config struct {
	Namespace string // 命名空间
	Subsystem string // 子系统
}

// New 创建指标收集器
func New(cfg *Config) *Metrics {
	if cfg == nil {
		cfg = &Config{}
	}

	registry := prometheus.NewRegistry()

	// 注册Go运行时指标
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{
		registry: registry,

		// HTTP指标
		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "path", "status"},
		),
		httpRequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "http_requests_in_flight",
				Help:      "Number of HTTP requests currently being processed",
			},
		),

		// gRPC指标
		grpcRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "grpc_requests_total",
				Help:      "Total number of gRPC requests",
			},
			[]string{"method", "code"},
		),
		grpcRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "grpc_request_duration_seconds",
				Help:      "gRPC request duration in seconds",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "code"},
		),

		// 业务指标
		businessCounter: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "business_operations_total",
				Help:      "Total number of business operations",
			},
			[]string{"operation", "status"},
		),
		businessGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "business_gauge",
				Help:      "Business gauge metrics",
			},
			[]string{"name"},
		),
		businessHistogram: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: cfg.Namespace,
				Subsystem: cfg.Subsystem,
				Name:      "business_duration_seconds",
				Help:      "Business operation duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
	}

	// 注册所有指标
	registry.MustRegister(
		m.httpRequestsTotal,
		m.httpRequestDuration,
		m.httpRequestsInFlight,
		m.grpcRequestsTotal,
		m.grpcRequestDuration,
		m.businessCounter,
		m.businessGauge,
		m.businessHistogram,
	)

	return m
}

// Handler 返回Prometheus HTTP处理器
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Registry 返回Registry
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// RecordHTTPRequest 记录HTTP请求
func (m *Metrics) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	status := strconv.Itoa(statusCode)
	m.httpRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.httpRequestDuration.WithLabelValues(method, path, status).Observe(duration.Seconds())
}

// HTTPRequestStart 标记HTTP请求开始
func (m *Metrics) HTTPRequestStart() {
	m.httpRequestsInFlight.Inc()
}

// HTTPRequestEnd 标记HTTP请求结束
func (m *Metrics) HTTPRequestEnd() {
	m.httpRequestsInFlight.Dec()
}

// RecordGRPCRequest 记录gRPC请求
func (m *Metrics) RecordGRPCRequest(method, code string, duration time.Duration) {
	m.grpcRequestsTotal.WithLabelValues(method, code).Inc()
	m.grpcRequestDuration.WithLabelValues(method, code).Observe(duration.Seconds())
}

// IncBusinessCounter 增加业务计数器
func (m *Metrics) IncBusinessCounter(operation, status string) {
	m.businessCounter.WithLabelValues(operation, status).Inc()
}

// SetBusinessGauge 设置业务仪表值
func (m *Metrics) SetBusinessGauge(name string, value float64) {
	m.businessGauge.WithLabelValues(name).Set(value)
}

// ObserveBusinessDuration 观察业务操作耗时
func (m *Metrics) ObserveBusinessDuration(operation string, duration time.Duration) {
	m.businessHistogram.WithLabelValues(operation).Observe(duration.Seconds())
}

// HTTPMiddleware HTTP指标中间件
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.HTTPRequestStart()
		defer m.HTTPRequestEnd()

		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		m.RecordHTTPRequest(r.Method, r.URL.Path, wrapped.statusCode, time.Since(start))
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
