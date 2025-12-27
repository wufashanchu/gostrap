package log

import (
	"context"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Level 日志级别
type Level = zapcore.Level

const (
	DebugLevel = zapcore.DebugLevel
	InfoLevel  = zapcore.InfoLevel
	WarnLevel  = zapcore.WarnLevel
	ErrorLevel = zapcore.ErrorLevel
	PanicLevel = zapcore.PanicLevel
	FatalLevel = zapcore.FatalLevel
)

// Logger 日志接口
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	With(fields ...Field) Logger
	WithContext(ctx context.Context) Logger
	Sync() error
}

// Field 日志字段
type Field = zap.Field

// 常用字段构造函数
var (
	String   = zap.String
	Int      = zap.Int
	Int64    = zap.Int64
	Float64  = zap.Float64
	Bool     = zap.Bool
	Any      = zap.Any
	Err      = zap.Error
	Duration = zap.Duration
	Time     = zap.Time
)

// Config 日志配置
type Config struct {
	Level      string `json:"level" yaml:"level"`             // 日志级别
	Format     string `json:"format" yaml:"format"`           // 输出格式: json, console
	Filename   string `json:"filename" yaml:"filename"`       // 日志文件路径
	MaxSize    int    `json:"max_size" yaml:"max_size"`       // 单文件最大大小(MB)
	MaxBackups int    `json:"max_backups" yaml:"max_backups"` // 最大备份数
	MaxAge     int    `json:"max_age" yaml:"max_age"`         // 最大保留天数
	Compress   bool   `json:"compress" yaml:"compress"`       // 是否压缩
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Level:      "info",
		Format:     "json",
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     7,
		Compress:   true,
	}
}

type logger struct {
	zap    *zap.Logger
	sugar  *zap.SugaredLogger
	config *Config
}

var (
	globalLogger Logger
	once         sync.Once
)

// Init 初始化全局日志
func Init(cfg *Config) Logger {
	once.Do(func() {
		globalLogger = New(cfg)
	})
	return globalLogger
}

// New 创建新的日志实例
func New(cfg *Config) Logger {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 解析日志级别
	level := parseLevel(cfg.Level)

	// 编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 创建编码器
	var encoder zapcore.Encoder
	if cfg.Format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 创建输出
	var writeSyncer zapcore.WriteSyncer
	if cfg.Filename != "" {
		writeSyncer = zapcore.NewMultiWriteSyncer(
			zapcore.AddSync(os.Stdout),
			zapcore.AddSync(&lumberjack.Logger{
				Filename:   cfg.Filename,
				MaxSize:    cfg.MaxSize,
				MaxBackups: cfg.MaxBackups,
				MaxAge:     cfg.MaxAge,
				Compress:   cfg.Compress,
			}),
		)
	} else {
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// 创建核心
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// 创建logger
	zapLogger := zap.New(core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)

	return &logger{
		zap:    zapLogger,
		sugar:  zapLogger.Sugar(),
		config: cfg,
	}
}

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

func (l *logger) Debug(msg string, fields ...Field) {
	l.zap.Debug(msg, fields...)
}

func (l *logger) Info(msg string, fields ...Field) {
	l.zap.Info(msg, fields...)
}

func (l *logger) Warn(msg string, fields ...Field) {
	l.zap.Warn(msg, fields...)
}

func (l *logger) Error(msg string, fields ...Field) {
	l.zap.Error(msg, fields...)
}

func (l *logger) Fatal(msg string, fields ...Field) {
	l.zap.Fatal(msg, fields...)
}

func (l *logger) With(fields ...Field) Logger {
	return &logger{
		zap:    l.zap.With(fields...),
		sugar:  l.sugar.With(fields),
		config: l.config,
	}
}

// WithContext 从context提取trace信息
func (l *logger) WithContext(ctx context.Context) Logger {
	if ctx == nil {
		return l
	}
	// 从context中提取trace_id等信息
	fields := extractTraceFields(ctx)
	if len(fields) > 0 {
		return l.With(fields...)
	}
	return l
}

func (l *logger) Sync() error {
	return l.zap.Sync()
}

// extractTraceFields 从context提取追踪字段
func extractTraceFields(ctx context.Context) []Field {
	var fields []Field
	// 预留trace_id等字段提取
	if traceID := ctx.Value("trace_id"); traceID != nil {
		if id, ok := traceID.(string); ok {
			fields = append(fields, String("trace_id", id))
		}
	}
	if spanID := ctx.Value("span_id"); spanID != nil {
		if id, ok := spanID.(string); ok {
			fields = append(fields, String("span_id", id))
		}
	}
	return fields
}

// 全局日志函数
func Debug(msg string, fields ...Field) { globalLogger.Debug(msg, fields...) }
func Info(msg string, fields ...Field)  { globalLogger.Info(msg, fields...) }
func Warn(msg string, fields ...Field)  { globalLogger.Warn(msg, fields...) }
func Error(msg string, fields ...Field) { globalLogger.Error(msg, fields...) }
func Fatal(msg string, fields ...Field) { globalLogger.Fatal(msg, fields...) }
func With(fields ...Field) Logger       { return globalLogger.With(fields...) }
func Sync() error                       { return globalLogger.Sync() }
