package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Error 业务错误
type Error struct {
	Code     int               `json:"code"`     // 业务错误码
	Message  string            `json:"message"`  // 错误消息
	HTTPCode int               `json:"-"`        // HTTP 状态码
	Reason   string            `json:"reason"`   // 错误原因(用于客户端判断)
	Metadata map[string]string `json:"metadata"` // 附加元数据
	cause    error             // 原始错误
}

// Error 实现error接口
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("[%d] %s: %s: %v", e.Code, e.Reason, e.Message, e.cause)
	}
	return fmt.Sprintf("[%d] %s: %s", e.Code, e.Reason, e.Message)
}

// Unwrap 返回原始错误
func (e *Error) Unwrap() error {
	return e.cause
}

// Is 判断错误类型
func (e *Error) Is(target error) bool {
	var t *Error
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// WithCause 添加原始错误
func (e *Error) WithCause(cause error) *Error {
	err := Clone(e)
	err.cause = cause
	return err
}

// WithMetadata 添加元数据
func (e *Error) WithMetadata(key, value string) *Error {
	err := Clone(e)
	if err.Metadata == nil {
		err.Metadata = make(map[string]string)
	}
	err.Metadata[key] = value
	return err
}

// Clone 克隆错误
func Clone(e *Error) *Error {
	metadata := make(map[string]string, len(e.Metadata))
	for k, v := range e.Metadata {
		metadata[k] = v
	}
	return &Error{
		Code:     e.Code,
		Message:  e.Message,
		HTTPCode: e.HTTPCode,
		Reason:   e.Reason,
		Metadata: metadata,
		cause:    e.cause,
	}
}

// New 创建新错误
func New(code int, reason, message string) *Error {
	return &Error{
		Code:     code,
		Message:  message,
		HTTPCode: http.StatusInternalServerError,
		Reason:   reason,
	}
}

// Newf 创建格式化错误
func Newf(code int, reason, format string, args ...interface{}) *Error {
	return &Error{
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
		HTTPCode: http.StatusInternalServerError,
		Reason:   reason,
	}
}

// FromError 从error转换
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return ErrUnknown.WithCause(err)
}

// Code 获取错误码
func Code(err error) int {
	if err == nil {
		return 0
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ErrUnknown.Code
}

// Reason 获取错误原因
func Reason(err error) string {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ErrUnknown.Reason
}

// 预定义错误码
const (
	CodeSuccess            = 0
	CodeUnknown            = 10000
	CodeInvalidArgument    = 10001
	CodeNotFound           = 10002
	CodeAlreadyExists      = 10003
	CodePermissionDenied   = 10004
	CodeUnauthenticated    = 10005
	CodeResourceExhausted  = 10006
	CodeFailedPrecondition = 10007
	CodeAborted            = 10008
	CodeOutOfRange         = 10009
	CodeUnimplemented      = 10010
	CodeInternal           = 10011
	CodeUnavailable        = 10012
	CodeDataLoss           = 10013
)

// 预定义错误
var (
	ErrUnknown = &Error{
		Code:     CodeUnknown,
		HTTPCode: http.StatusInternalServerError,
		Reason:   "UNKNOWN",
		Message:  "unknown error",
	}
	ErrInvalidArgument = &Error{
		Code:     CodeInvalidArgument,
		HTTPCode: http.StatusBadRequest,
		Reason:   "INVALID_ARGUMENT",
		Message:  "invalid argument",
	}
	ErrNotFound = &Error{
		Code:     CodeNotFound,
		HTTPCode: http.StatusNotFound,
		Reason:   "NOT_FOUND",
		Message:  "resource not found",
	}
	ErrAlreadyExists = &Error{
		Code:     CodeAlreadyExists,
		HTTPCode: http.StatusConflict,
		Reason:   "ALREADY_EXISTS",
		Message:  "resource already exists",
	}
	ErrPermissionDenied = &Error{
		Code:     CodePermissionDenied,
		HTTPCode: http.StatusForbidden,
		Reason:   "PERMISSION_DENIED",
		Message:  "permission denied",
	}
	ErrUnauthenticated = &Error{
		Code:     CodeUnauthenticated,
		HTTPCode: http.StatusUnauthorized,
		Reason:   "UNAUTHENTICATED",
		Message:  "unauthenticated",
	}
	ErrResourceExhausted = &Error{
		Code:     CodeResourceExhausted,
		HTTPCode: http.StatusTooManyRequests,
		Reason:   "RESOURCE_EXHAUSTED",
		Message:  "resource exhausted",
	}
	ErrInternal = &Error{
		Code:     CodeInternal,
		HTTPCode: http.StatusInternalServerError,
		Reason:   "INTERNAL",
		Message:  "internal error",
	}
	ErrUnavailable = &Error{
		Code:     CodeUnavailable,
		HTTPCode: http.StatusServiceUnavailable,
		Reason:   "UNAVAILABLE",
		Message:  "service unavailable",
	}
)

// IsNotFound 判断是否为NotFound错误
func IsNotFound(err error) bool {
	return Code(err) == CodeNotFound
}

// IsInvalidArgument 判断是否为InvalidArgument错误
func IsInvalidArgument(err error) bool {
	return Code(err) == CodeInvalidArgument
}

// IsUnauthenticated 判断是否为Unauthenticated错误
func IsUnauthenticated(err error) bool {
	return Code(err) == CodeUnauthenticated
}

// IsPermissionDenied 判断是否为PermissionDenied错误
func IsPermissionDenied(err error) bool {
	return Code(err) == CodePermissionDenied
}

// IsInternal 判断是否为Internal错误
func IsInternal(err error) bool {
	return Code(err) == CodeInternal
}
