package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// Config 追踪配置
type Config struct {
	ServiceName    string  `json:"service_name" yaml:"service_name"`
	ServiceVersion string  `json:"service_version" yaml:"service_version"`
	Environment    string  `json:"environment" yaml:"environment"`
	Endpoint       string  `json:"endpoint" yaml:"endpoint"` // OTLP collector endpoint
	SampleRate     float64 `json:"sample_rate" yaml:"sample_rate"`
	Insecure       bool    `json:"insecure" yaml:"insecure"`
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		ServiceName:    "gostrap-service",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		Endpoint:       "localhost:4317",
		SampleRate:     1.0,
		Insecure:       true,
	}
}

// Provider 追踪提供器
type Provider struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
	config *Config
}

// NewProvider 创建追踪提供器
func NewProvider(cfg *Config) (*Provider, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	ctx := context.Background()

	// 创建OTLP导出器
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, err
	}

	// 创建资源
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	// 创建采样器
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// 创建TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// 设置全局TracerProvider
	otel.SetTracerProvider(tp)

	// 设置全局传播器
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{
		tp:     tp,
		tracer: tp.Tracer(cfg.ServiceName),
		config: cfg,
	}, nil
}

// Tracer 获取追踪器
func (p *Provider) Tracer() trace.Tracer {
	return p.tracer
}

// Shutdown 关闭追踪提供器
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.tp.Shutdown(ctx)
}

// StartSpan 开始一个新的Span
func (p *Provider) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return p.tracer.Start(ctx, name, opts...)
}

// SpanFromContext 从context获取Span
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceIDFromContext 从context获取TraceID
func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasTraceID() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// SpanIDFromContext 从context获取SpanID
func SpanIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().HasSpanID() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// SetSpanAttributes 设置Span属性
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

// SetSpanError 设置Span错误
func SetSpanError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
}

// Span辅助函数
type SpanHelper struct {
	ctx  context.Context
	span trace.Span
}

// NewSpan 创建Span辅助器
func NewSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) *SpanHelper {
	ctx, span := otel.Tracer("").Start(ctx, name, opts...)
	return &SpanHelper{ctx: ctx, span: span}
}

// Context 返回context
func (s *SpanHelper) Context() context.Context {
	return s.ctx
}

// End 结束Span
func (s *SpanHelper) End() {
	s.span.End()
}

// SetAttributes 设置属性
func (s *SpanHelper) SetAttributes(attrs ...attribute.KeyValue) *SpanHelper {
	s.span.SetAttributes(attrs...)
	return s
}

// RecordError 记录错误
func (s *SpanHelper) RecordError(err error) *SpanHelper {
	s.span.RecordError(err)
	return s
}

// AddEvent 添加事件
func (s *SpanHelper) AddEvent(name string, attrs ...attribute.KeyValue) *SpanHelper {
	s.span.AddEvent(name, trace.WithAttributes(attrs...))
	return s
}

// TimeOperation 计时操作
func TimeOperation(ctx context.Context, name string, fn func(context.Context) error) error {
	ctx, span := otel.Tracer("").Start(ctx, name)
	defer span.End()

	start := time.Now()
	err := fn(ctx)
	duration := time.Since(start)

	span.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))
	if err != nil {
		span.RecordError(err)
	}

	return err
}

// TimeOperationWithResult 带返回值的计时操作
func TimeOperationWithResult[T any](ctx context.Context, name string, fn func(context.Context) (T, error)) (T, error) {
	ctx, span := otel.Tracer("").Start(ctx, name)
	defer span.End()

	start := time.Now()
	result, err := fn(ctx)
	duration := time.Since(start)

	span.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))
	if err != nil {
		span.RecordError(err)
	}

	return result, err
}

// HTTPSpan HTTP请求追踪Span
type HTTPSpan struct {
	*SpanHelper
}

// NewHTTPSpan 创建HTTP追踪Span
func NewHTTPSpan(ctx context.Context, method, path string) *HTTPSpan {
	spanName := method + " " + path
	helper := NewSpan(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
	helper.SetAttributes(
		semconv.HTTPMethod(method),
		semconv.HTTPTarget(path),
	)
	return &HTTPSpan{SpanHelper: helper}
}

// SetHTTPStatus 设置HTTP状态码
func (h *HTTPSpan) SetHTTPStatus(statusCode int) *HTTPSpan {
	h.span.SetAttributes(semconv.HTTPStatusCode(statusCode))
	return h
}

// SetHTTPRequestSize 设置请求大小
func (h *HTTPSpan) SetHTTPRequestSize(size int64) *HTTPSpan {
	h.span.SetAttributes(attribute.Int64("http.request_content_length", size))
	return h
}

// SetHTTPResponseSize 设置响应大小
func (h *HTTPSpan) SetHTTPResponseSize(size int64) *HTTPSpan {
	h.span.SetAttributes(attribute.Int64("http.response_content_length", size))
	return h
}

// SetHTTPRoute 设置HTTP路由
func (h *HTTPSpan) SetHTTPRoute(route string) *HTTPSpan {
	h.span.SetAttributes(semconv.HTTPRoute(route))
	return h
}

// SetHTTPClientIP 设置客户端IP
func (h *HTTPSpan) SetHTTPClientIP(ip string) *HTTPSpan {
	h.span.SetAttributes(attribute.String("http.client_ip", ip))
	return h
}

// SetHTTPUserAgent 设置User-Agent
func (h *HTTPSpan) SetHTTPUserAgent(ua string) *HTTPSpan {
	h.span.SetAttributes(attribute.String("http.user_agent", ua))
	return h
}

// DBSpan 数据库追踪Span
type DBSpan struct {
	*SpanHelper
}

// NewDBSpan 创建数据库追踪Span
func NewDBSpan(ctx context.Context, operation, dbSystem string) *DBSpan {
	helper := NewSpan(ctx, operation, trace.WithSpanKind(trace.SpanKindClient))
	helper.SetAttributes(
		semconv.DBSystemKey.String(dbSystem),
		semconv.DBOperation(operation),
	)
	return &DBSpan{SpanHelper: helper}
}

// SetDBStatement 设置SQL语句
func (d *DBSpan) SetDBStatement(statement string) *DBSpan {
	d.span.SetAttributes(semconv.DBStatement(statement))
	return d
}

// SetDBName 设置数据库名称
func (d *DBSpan) SetDBName(name string) *DBSpan {
	d.span.SetAttributes(semconv.DBName(name))
	return d
}

// SetDBTable 设置数据库表名
func (d *DBSpan) SetDBTable(table string) *DBSpan {
	d.span.SetAttributes(semconv.DBSQLTable(table))
	return d
}

// SetDBRowsAffected 设置受影响的行数
func (d *DBSpan) SetDBRowsAffected(count int64) *DBSpan {
	d.span.SetAttributes(attribute.Int64("db.rows_affected", count))
	return d
}

// SetDBConnectionString 设置连接字符串（注意脱敏）
func (d *DBSpan) SetDBConnectionString(connStr string) *DBSpan {
	d.span.SetAttributes(semconv.DBConnectionString(connStr))
	return d
}

// RPCSpan RPC调用追踪Span
type RPCSpan struct {
	*SpanHelper
}

// NewRPCSpan 创建RPC追踪Span（作为客户端）
func NewRPCSpan(ctx context.Context, service, method string) *RPCSpan {
	spanName := service + "/" + method
	helper := NewSpan(ctx, spanName, trace.WithSpanKind(trace.SpanKindClient))
	helper.SetAttributes(
		semconv.RPCService(service),
		semconv.RPCMethod(method),
		semconv.RPCSystemGRPC,
	)
	return &RPCSpan{SpanHelper: helper}
}

// NewRPCServerSpan 创建RPC追踪Span（作为服务端）
func NewRPCServerSpan(ctx context.Context, service, method string) *RPCSpan {
	spanName := service + "/" + method
	helper := NewSpan(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
	helper.SetAttributes(
		semconv.RPCService(service),
		semconv.RPCMethod(method),
		semconv.RPCSystemGRPC,
	)
	return &RPCSpan{SpanHelper: helper}
}

// SetRPCStatus 设置RPC状态
func (r *RPCSpan) SetRPCStatus(code int64) *RPCSpan {
	r.span.SetAttributes(semconv.RPCGRPCStatusCodeKey.Int64(code))
	return r
}

// SetRPCMessageType 设置消息类型
func (r *RPCSpan) SetRPCMessageType(msgType string) *RPCSpan {
	r.span.SetAttributes(attribute.String("rpc.message.type", msgType))
	return r
}

// SetRPCMessageID 设置消息ID
func (r *RPCSpan) SetRPCMessageID(id int64) *RPCSpan {
	r.span.SetAttributes(attribute.Int64("rpc.message.id", id))
	return r
}

// MessageSpan 消息队列追踪Span
type MessageSpan struct {
	*SpanHelper
}

// NewMessageProducerSpan 创建消息生产者Span
func NewMessageProducerSpan(ctx context.Context, system, destination string) *MessageSpan {
	spanName := destination + " send"
	helper := NewSpan(ctx, spanName, trace.WithSpanKind(trace.SpanKindProducer))
	helper.SetAttributes(
		semconv.MessagingSystem(system),
		semconv.MessagingDestinationName(destination),
		semconv.MessagingOperationPublish,
	)
	return &MessageSpan{SpanHelper: helper}
}

// NewMessageConsumerSpan 创建消息消费者Span
func NewMessageConsumerSpan(ctx context.Context, system, destination string) *MessageSpan {
	spanName := destination + " receive"
	helper := NewSpan(ctx, spanName, trace.WithSpanKind(trace.SpanKindConsumer))
	helper.SetAttributes(
		semconv.MessagingSystem(system),
		semconv.MessagingDestinationName(destination),
		semconv.MessagingOperationReceive,
	)
	return &MessageSpan{SpanHelper: helper}
}

// SetMessageID 设置消息ID
func (m *MessageSpan) SetMessageID(id string) *MessageSpan {
	m.span.SetAttributes(semconv.MessagingMessageID(id))
	return m
}

// SetMessagePayloadSize 设置消息大小
func (m *MessageSpan) SetMessagePayloadSize(size int64) *MessageSpan {
	m.span.SetAttributes(attribute.Int64("messaging.message.payload_size_bytes", size))
	return m
}

// SetMessageConversationID 设置会话ID
func (m *MessageSpan) SetMessageConversationID(id string) *MessageSpan {
	m.span.SetAttributes(semconv.MessagingMessageConversationID(id))
	return m
}

// CacheSpan 缓存追踪Span
type CacheSpan struct {
	*SpanHelper
}

// NewCacheSpan 创建缓存追踪Span
func NewCacheSpan(ctx context.Context, operation, system string) *CacheSpan {
	helper := NewSpan(ctx, operation, trace.WithSpanKind(trace.SpanKindClient))
	helper.SetAttributes(
		attribute.String("cache.system", system),
		attribute.String("cache.operation", operation),
	)
	return &CacheSpan{SpanHelper: helper}
}

// SetCacheKey 设置缓存键
func (c *CacheSpan) SetCacheKey(key string) *CacheSpan {
	c.span.SetAttributes(attribute.String("cache.key", key))
	return c
}

// SetCacheHit 设置是否命中缓存
func (c *CacheSpan) SetCacheHit(hit bool) *CacheSpan {
	c.span.SetAttributes(attribute.Bool("cache.hit", hit))
	return c
}

// SetCacheTTL 设置缓存TTL
func (c *CacheSpan) SetCacheTTL(ttl time.Duration) *CacheSpan {
	c.span.SetAttributes(attribute.Int64("cache.ttl_ms", ttl.Milliseconds()))
	return c
}

// InjectContext 将追踪上下文注入到载体中
func InjectContext(ctx context.Context, carrier propagation.TextMapCarrier) {
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// ExtractContext 从载体中提取追踪上下文
func ExtractContext(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// MapCarrier 简单的map载体实现
type MapCarrier map[string]string

// Get 获取值
func (c MapCarrier) Get(key string) string {
	return c[key]
}

// Set 设置值
func (c MapCarrier) Set(key, value string) {
	c[key] = value
}

// Keys 获取所有键
func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// WithSpan 便捷函数：执行带Span的操作
func WithSpan(ctx context.Context, name string, fn func(context.Context) error, opts ...trace.SpanStartOption) error {
	ctx, span := otel.Tracer("").Start(ctx, name, opts...)
	defer span.End()

	if err := fn(ctx); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

// WithSpanResult 便捷函数：执行带Span和返回值的操作
func WithSpanResult[T any](ctx context.Context, name string, fn func(context.Context) (T, error), opts ...trace.SpanStartOption) (T, error) {
	ctx, span := otel.Tracer("").Start(ctx, name, opts...)
	defer span.End()

	result, err := fn(ctx)
	if err != nil {
		span.RecordError(err)
	}
	return result, err
}

// IsTracingEnabled 检查追踪是否启用
func IsTracingEnabled(ctx context.Context) bool {
	span := trace.SpanFromContext(ctx)
	return span.SpanContext().IsSampled()
}

// GetTraceParent 获取traceparent头部值
func GetTraceParent(ctx context.Context) string {
	carrier := MapCarrier{}
	InjectContext(ctx, carrier)
	return carrier.Get("traceparent")
}

// GetTraceState 获取tracestate头部值
func GetTraceState(ctx context.Context) string {
	carrier := MapCarrier{}
	InjectContext(ctx, carrier)
	return carrier.Get("tracestate")
}
