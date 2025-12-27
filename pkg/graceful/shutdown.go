package graceful

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/wufashanchu/gostrap/pkg/log"
)

// ShutdownFunc 关闭函数
type ShutdownFunc func(ctx context.Context) error

// Manager 优雅关闭管理器
type Manager struct {
	timeout   time.Duration
	callbacks []ShutdownFunc
	mu        sync.Mutex
	logger    log.Logger
}

// NewManager 创建关闭管理器
func NewManager(timeout time.Duration, logger log.Logger) *Manager {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Manager{
		timeout: timeout,
		logger:  logger,
	}
}

// Register 注册关闭回调
func (m *Manager) Register(fn ShutdownFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, fn)
}

// RegisterWithName 注册带名称的关闭回调
func (m *Manager) RegisterWithName(name string, fn ShutdownFunc) {
	m.Register(func(ctx context.Context) error {
		m.logger.Info("shutting down component", log.String("component", name))
		return fn(ctx)
	})
}

// Wait 等待关闭信号
func (m *Manager) Wait() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	sig := <-quit
	m.logger.Info("received shutdown signal", log.String("signal", sig.String()))

	m.Shutdown()
}

// Shutdown 执行关闭
func (m *Manager) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	m.mu.Lock()
	callbacks := make([]ShutdownFunc, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.mu.Unlock()

	// 逆序执行关闭回调(后注册的先关闭)
	var wg sync.WaitGroup
	errChan := make(chan error, len(callbacks))

	for i := len(callbacks) - 1; i >= 0; i-- {
		wg.Add(1)
		fn := callbacks[i]
		go func() {
			defer wg.Done()
			if err := fn(ctx); err != nil {
				errChan <- err
			}
		}()
	}

	// 等待所有回调完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("graceful shutdown completed")
	case <-ctx.Done():
		m.logger.Warn("graceful shutdown timeout, forcing exit")
	}

	close(errChan)
	for err := range errChan {
		m.logger.Error("shutdown error", log.Err(err))
	}
}

// ShutdownHook 全局关闭钩子
type ShutdownHook struct {
	manager *Manager
	once    sync.Once
}

var defaultHook = &ShutdownHook{}

// SetupGracefulShutdown 设置全局优雅关闭
func SetupGracefulShutdown(timeout time.Duration, logger log.Logger) *Manager {
	defaultHook.once.Do(func() {
		defaultHook.manager = NewManager(timeout, logger)
	})
	return defaultHook.manager
}

// Register 注册全局关闭回调
func Register(fn ShutdownFunc) {
	if defaultHook.manager != nil {
		defaultHook.manager.Register(fn)
	}
}

// RegisterWithName 注册带名称的全局关闭回调
func RegisterWithName(name string, fn ShutdownFunc) {
	if defaultHook.manager != nil {
		defaultHook.manager.RegisterWithName(name, fn)
	}
}

// Wait 等待全局关闭信号
func Wait() {
	if defaultHook.manager != nil {
		defaultHook.manager.Wait()
	}
}
