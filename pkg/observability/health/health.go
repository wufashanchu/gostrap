package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status 健康状态
type Status string

const (
	StatusUp      Status = "UP"
	StatusDown    Status = "DOWN"
	StatusUnknown Status = "UNKNOWN"
)

// Check 健康检查项
type Check struct {
	Name   string                 `json:"name"`
	Status Status                 `json:"status"`
	Time   time.Time              `json:"time"`
	Data   map[string]interface{} `json:"data,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

// Result 健康检查结果
type Result struct {
	Status  Status  `json:"status"`
	Version string  `json:"version,omitempty"`
	Checks  []Check `json:"checks,omitempty"`
}

// Checker 健康检查器接口
type Checker interface {
	Name() string
	Check(ctx context.Context) Check
}

// Handler 健康检查处理器
type Handler struct {
	version  string
	checkers []Checker
	mu       sync.RWMutex
	timeout  time.Duration
	ready    bool
	readyMu  sync.RWMutex
}

// NewHandler 创建健康检查处理器
func NewHandler(version string) *Handler {
	return &Handler{
		version: version,
		timeout: 5 * time.Second,
		ready:   false,
	}
}

// Register 注册健康检查器
func (h *Handler) Register(checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers = append(h.checkers, checker)
}

// SetReady 设置就绪状态
func (h *Handler) SetReady(ready bool) {
	h.readyMu.Lock()
	defer h.readyMu.Unlock()
	h.ready = ready
}

// IsReady 获取就绪状态
func (h *Handler) IsReady() bool {
	h.readyMu.RLock()
	defer h.readyMu.RUnlock()
	return h.ready
}

// SetTimeout 设置检查超时
func (h *Handler) SetTimeout(timeout time.Duration) {
	h.timeout = timeout
}

// Check 执行健康检查
func (h *Handler) Check(ctx context.Context) Result {
	h.mu.RLock()
	checkers := make([]Checker, len(h.checkers))
	copy(checkers, h.checkers)
	h.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	result := Result{
		Status:  StatusUp,
		Version: h.version,
		Checks:  make([]Check, 0, len(checkers)),
	}

	var wg sync.WaitGroup
	checkResults := make(chan Check, len(checkers))

	for _, checker := range checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()
			checkResults <- c.Check(ctx)
		}(checker)
	}

	go func() {
		wg.Wait()
		close(checkResults)
	}()

	for check := range checkResults {
		result.Checks = append(result.Checks, check)
		if check.Status != StatusUp {
			result.Status = StatusDown
		}
	}

	return result
}

// LivenessHandler Kubernetes存活探针处理器
func (h *Handler) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := h.Check(r.Context())
		h.writeResponse(w, result)
	}
}

// ReadinessHandler Kubernetes就绪探针处理器
func (h *Handler) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.IsReady() {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(Result{Status: StatusDown})
			return
		}

		result := h.Check(r.Context())
		h.writeResponse(w, result)
	}
}

// StartupHandler Kubernetes启动探针处理器
func (h *Handler) StartupHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.IsReady() {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(Result{Status: StatusDown})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Result{Status: StatusUp})
	}
}

func (h *Handler) writeResponse(w http.ResponseWriter, result Result) {
	w.Header().Set("Content-Type", "application/json")
	if result.Status != StatusUp {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(result)
}

// DatabaseChecker 数据库健康检查器
type DatabaseChecker struct {
	name string
	ping func(ctx context.Context) error
}

// NewDatabaseChecker 创建数据库检查器
func NewDatabaseChecker(name string, ping func(ctx context.Context) error) *DatabaseChecker {
	return &DatabaseChecker{name: name, ping: ping}
}

func (c *DatabaseChecker) Name() string {
	return c.name
}

func (c *DatabaseChecker) Check(ctx context.Context) Check {
	check := Check{
		Name: c.name,
		Time: time.Now(),
	}
	if err := c.ping(ctx); err != nil {
		check.Status = StatusDown
		check.Error = err.Error()
	} else {
		check.Status = StatusUp
	}
	return check
}

// RedisChecker Redis健康检查器
type RedisChecker struct {
	name string
	ping func(ctx context.Context) error
}

// NewRedisChecker 创建Redis检查器
func NewRedisChecker(name string, ping func(ctx context.Context) error) *RedisChecker {
	return &RedisChecker{name: name, ping: ping}
}

func (c *RedisChecker) Name() string {
	return c.name
}

func (c *RedisChecker) Check(ctx context.Context) Check {
	check := Check{
		Name: c.name,
		Time: time.Now(),
	}
	if err := c.ping(ctx); err != nil {
		check.Status = StatusDown
		check.Error = err.Error()
	} else {
		check.Status = StatusUp
	}
	return check
}

// CustomChecker 自定义检查器
type CustomChecker struct {
	name    string
	checkFn func(ctx context.Context) (map[string]interface{}, error)
}

// NewCustomChecker 创建自定义检查器
func NewCustomChecker(name string, checkFn func(ctx context.Context) (map[string]interface{}, error)) *CustomChecker {
	return &CustomChecker{name: name, checkFn: checkFn}
}

func (c *CustomChecker) Name() string {
	return c.name
}

func (c *CustomChecker) Check(ctx context.Context) Check {
	check := Check{
		Name: c.name,
		Time: time.Now(),
	}
	data, err := c.checkFn(ctx)
	if err != nil {
		check.Status = StatusDown
		check.Error = err.Error()
	} else {
		check.Status = StatusUp
		check.Data = data
	}
	return check
}
