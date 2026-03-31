package actor

import (
	"fmt"
	"log"
	"os"
	"sync"
)

// LogLevel 定义日志级别
type LogLevel int

const (
	// LogLevelNone 禁用所有日志
	LogLevelNone LogLevel = iota
	// LogLevelError 只记录错误
	LogLevelError
	// LogLevelWarn 记录错误和警告
	LogLevelWarn
	// LogLevelInfo 记录错误、警告和信息
	LogLevelInfo
	// LogLevelDebug 记录所有日志（包括调试信息）
	LogLevelDebug
)

// FutureLogger 定义Future的日志接口
type FutureLogger interface {
	// Debug 记录调试日志
	Debug(format string, args ...interface{})

	// Info 记录信息日志
	Info(format string, args ...interface{})

	// Warn 记录警告日志
	Warn(format string, args ...interface{})

	// Error 记录错误日志
	Error(format string, args ...interface{})
}

// defaultFutureLogger 默认日志实现
type defaultFutureLogger struct {
	logger *log.Logger
	level  LogLevel
	mu     sync.Mutex
}

// newDefaultFutureLogger 创建默认日志器
func newDefaultFutureLogger(level LogLevel) *defaultFutureLogger {
	return &defaultFutureLogger{
		logger: log.New(os.Stdout, "[Future] ", log.LstdFlags|log.Lmicroseconds),
		level:  level,
	}
}

// Debug 记录调试日志
func (l *defaultFutureLogger) Debug(format string, args ...interface{}) {
	if l.level >= LogLevelDebug {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.logger.SetPrefix("[Future][DEBUG] ")
		l.logger.Printf(format, args...)
	}
}

// Info 记录信息日志
func (l *defaultFutureLogger) Info(format string, args ...interface{}) {
	if l.level >= LogLevelInfo {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.logger.SetPrefix("[Future][INFO] ")
		l.logger.Printf(format, args...)
	}
}

// Warn 记录警告日志
func (l *defaultFutureLogger) Warn(format string, args ...interface{}) {
	if l.level >= LogLevelWarn {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.logger.SetPrefix("[Future][WARN] ")
		l.logger.Printf(format, args...)
	}
}

// Error 记录错误日志
func (l *defaultFutureLogger) Error(format string, args ...interface{}) {
	if l.level >= LogLevelError {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.logger.SetPrefix("[Future][ERROR] ")
		l.logger.Printf(format, args...)
	}
}

// 设置全局Future日志器
var (
	globalFutureLogger FutureLogger = newDefaultFutureLogger(LogLevelNone) // 默认禁用日志
	loggerMutex        sync.RWMutex
)

// SetFutureLogger 设置全局Future日志器
func SetFutureLogger(logger FutureLogger) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	globalFutureLogger = logger
}

// SetFutureLogLevel 设置全局Future日志级别
func SetFutureLogLevel(level LogLevel) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()
	globalFutureLogger = newDefaultFutureLogger(level)
}

// GetFutureLogger 获取当前的Future日志器
func GetFutureLogger() FutureLogger {
	loggerMutex.RLock()
	defer loggerMutex.RUnlock()
	return globalFutureLogger
}

// futureLog 便捷的日志记录函数
func futureLog(level LogLevel, format string, args ...interface{}) {
	logger := GetFutureLogger()
	switch level {
	case LogLevelDebug:
		logger.Debug(format, args...)
	case LogLevelInfo:
		logger.Info(format, args...)
	case LogLevelWarn:
		logger.Warn(format, args...)
	case LogLevelError:
		logger.Error(format, args...)
	}
}

// Helper functions for common log messages

func logFutureCreated(correlationID, target string, timeoutMs int64) {
	futureLog(LogLevelDebug,
		"Future created: correlationID=%s, target=%s, timeout=%dms",
		correlationID, target, timeoutMs)
}

func logFutureCompleted(correlationID string, success bool, err error) {
	if err != nil {
		futureLog(LogLevelDebug,
			"Future completed with ERROR: correlationID=%s, error=%v",
			correlationID, err)
	} else {
		futureLog(LogLevelDebug,
			"Future completed SUCCESS: correlationID=%s",
			correlationID)
	}
}

func logFutureCancelled(correlationID string) {
	futureLog(LogLevelInfo,
		"Future cancelled: correlationID=%s",
		correlationID)
}

func logFutureTimeout(correlationID string, timeoutMs int64) {
	futureLog(LogLevelInfo,
		"Future timeout after %dms: correlationID=%s",
		timeoutMs, correlationID)
}

func logUnknownCorrelationID(correlationID string) {
	futureLog(LogLevelWarn,
		"Received response for unknown Future correlationID=%s - may be late response or already cleaned up",
		correlationID)
}

func logCallbackRegistered(correlationID string) {
	futureLog(LogLevelDebug,
		"Callback registered: correlationID=%s",
		correlationID)
}

func logCallbackInvoked(correlationID string, hasError bool) {
	if hasError {
		futureLog(LogLevelDebug,
			"Callback invoked with ERROR: correlationID=%s",
			correlationID)
	} else {
		futureLog(LogLevelDebug,
			"Callback invoked SUCCESS: correlationID=%s",
			correlationID)
	}
}

func logManagerCleanup(count int) {
	futureLog(LogLevelInfo,
		"FutureManager cleanup removed %d completed/timed-out futures",
		count)
}

func logManagerStats(activeCount, totalCreated, totalCompleted, totalCancelled, totalTimeout int) {
	futureLog(LogLevelInfo,
		"FutureManager stats: active=%d, totalCreated=%d, totalCompleted=%d, totalCancelled=%d, totalTimeout=%d",
		activeCount, totalCreated, totalCompleted, totalCancelled, totalTimeout)
}

// CustomFutureLogger 允许用户自定义日志实现
type CustomFutureLogger struct {
	DebugFunc func(format string, args ...interface{})
	InfoFunc  func(format string, args ...interface{})
	WarnFunc  func(format string, args ...interface{})
	ErrorFunc func(format string, args ...interface{})
}

// Debug 实现
func (l *CustomFutureLogger) Debug(format string, args ...interface{}) {
	if l.DebugFunc != nil {
		l.DebugFunc(format, args...)
	}
}

// Info 实现
func (l *CustomFutureLogger) Info(format string, args ...interface{}) {
	if l.InfoFunc != nil {
		l.InfoFunc(format, args...)
	}
}

// Warn 实现
func (l *CustomFutureLogger) Warn(format string, args ...interface{}) {
	if l.WarnFunc != nil {
		l.WarnFunc(format, args...)
	}
}

// Error 实现
func (l *CustomFutureLogger) Error(format string, args ...interface{}) {
	if l.ErrorFunc != nil {
		l.ErrorFunc(format, args...)
	}
}

// NewCustomLogger 创建自定义日志器
// 参数都是可选的，nil表示该级别的日志被禁用
func NewCustomLogger(
	debugFunc func(format string, args ...interface{}),
	infoFunc func(format string, args ...interface{}),
	warnFunc func(format string, args ...interface{}),
	errorFunc func(format string, args ...interface{}),
) FutureLogger {
	return &CustomFutureLogger{
		DebugFunc: debugFunc,
		InfoFunc:  infoFunc,
		WarnFunc:  warnFunc,
		ErrorFunc: errorFunc,
	}
}

// JSONFutureLogger 输出JSON格式的日志，适合日志聚合系统
type JSONFutureLogger struct {
	logger *log.Logger
	level  LogLevel
}

// NewJSONFutureLogger 创建JSON日志器
func NewJSONFutureLogger(output *os.File, level LogLevel) FutureLogger {
	prefix := ""
	return &JSONFutureLogger{
		logger: log.New(output, prefix, 0),
		level:  level,
	}
}

// Debug 实现
func (l *JSONFutureLogger) Debug(format string, args ...interface{}) {
	if l.level >= LogLevelDebug {
		l.log("debug", format, args...)
	}
}

// Info 实现
func (l *JSONFutureLogger) Info(format string, args ...interface{}) {
	if l.level >= LogLevelInfo {
		l.log("info", format, args...)
	}
}

// Warn 实现
func (l *JSONFutureLogger) Warn(format string, args ...interface{}) {
	if l.level >= LogLevelWarn {
		l.log("warn", format, args...)
	}
}

// Error 实现
func (l *JSONFutureLogger) Error(format string, args ...interface{}) {
	if l.level >= LogLevelError {
		l.log("error", format, args...)
	}
}

func (l *JSONFutureLogger) log(level, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.logger.Printf(`{"level":"%s","message":"%s"}`, level, message)
}
