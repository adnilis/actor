package actor

import (
	"net/http"
	_ "net/http/pprof" // Import pprof handlers
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

const (
	defaultProfilingPort = "6060"
)

var (
	pprofEnabled   bool
	pprofServer    *http.Server
	pprofStarted   bool
	pprofStartLock = make(chan struct{}, 1)
)

// ProfilingConfig holds profiling configuration
type ProfilingConfig struct {
	Enabled bool
	Port    string
}

// DefaultProfilingConfig returns default profiling configuration
func DefaultProfilingConfig() ProfilingConfig {
	return ProfilingConfig{
		Enabled: false,
		Port:    defaultProfilingPort,
	}
}

// ConfigureProfiling configures profiling based on environment variables or provided config
func ConfigureProfiling(config ProfilingConfig) {
	select {
	case <-pprofStartLock:
		// Already configured
		return
	default:
		pprofStartLock <- struct{}{}
	}

	// Check environment variables if not explicitly disabled
	if !config.Enabled {
		if os.Getenv("ACTOR_PPROF_ENABLE") == "1" {
			config.Enabled = true
		}
	}

	if !config.Enabled {
		pprofEnabled = false
		return
	}

	// Get port from environment if not specified
	if config.Port == "" {
		if port := os.Getenv("ACTOR_PPROF_PORT"); port != "" {
			config.Port = port
		} else {
			config.Port = defaultProfilingPort
		}
	}

	pprofEnabled = true

	// Start pprof server
	pprofServer = &http.Server{
		Addr: "127.0.0.1:" + config.Port, // Bind to localhost only for security
	}

	go func() {
		if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't panic
		}
	}()

	pprofStarted = true
}

// StopProfiling stops the pprof server
func StopProfiling() {
	if pprofServer != nil && pprofStarted {
		pprofServer.Close()
		pprofStarted = false
		pprofEnabled = false
	}
}

// IsProfilingEnabled returns true if profiling is enabled
func IsProfilingEnabled() bool {
	return pprofEnabled
}

// ProfilingInfo returns information about the profiling configuration
func ProfilingInfo() (enabled bool, port string, started bool) {
	return pprofEnabled, pprofServer.Addr, pprofStarted
}

// GetProfilingURL returns the URL for accessing profiling data
func GetProfilingURL(port string) string {
	if port == "" {
		port = defaultProfilingPort
	}
	return "http://127.0.0.1:" + port + "/debug/pprof/"
}

// StartCPUProfiling starts CPU profiling (manual trigger)
// Returns a stop function that should be called to stop profiling and write the profile
func StartCPUProfiling(filename string) func() error {
	if !pprofEnabled {
		return func() error {
			return os.ErrNotExist
		}
	}

	// Create the profile file
	f, err := os.Create(filename)
	if err != nil {
		// Return a no-op function that returns the error
		return func() error {
			return err
		}
	}

	// Start CPU profiling
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return func() error {
			return err
		}
	}

	// Return a stop function
	return func() error {
		pprof.StopCPUProfile()
		if err := f.Close(); err != nil {
			return err
		}
		return nil
	}
}

// StartCPUProfilingWithDuration starts CPU profiling for a specified duration
// Automatically stops profiling after the duration and writes to the specified file
func StartCPUProfilingWithDuration(filename string, duration time.Duration) error {
	if !pprofEnabled {
		return os.ErrNotExist
	}

	stopProfiling := StartCPUProfiling(filename)
	if stopProfiling == nil {
		return os.ErrNotExist
	}

	// Set up signal handler to allow early termination
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for duration or signal
	select {
	case <-sigChan:
		// User interrupted, stop profiling
	case <-time.After(duration):
		// Duration elapsed, stop profiling
	}

	// Stop profiling
	signal.Stop(sigChan)
	return stopProfiling()
}

// GetRuntimeStats returns current runtime statistics
func GetRuntimeStats() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"goroutines": runtime.NumGoroutine(),
		"threads":    runtime.NumCgoCall(),
		"cpu_count":  runtime.NumCPU(),
		"memory": map[string]interface{}{
			"alloc":       m.Alloc,
			"total_alloc": m.TotalAlloc,
			"sys":         m.Sys,
			"heap_alloc":  m.HeapAlloc,
			"heap_sys":    m.HeapSys,
			"heap_inuse":  m.HeapInuse,
			"heap_idle":   m.HeapIdle,
			"stack_inuse": m.StackInuse,
			"stack_sys":   m.StackSys,
		},
		"gc": map[string]interface{}{
			"next_gc":         m.NextGC,
			"last_gc":         m.LastGC,
			"pause_total_ns":  m.PauseTotalNs,
			"num_gc":          m.NumGC,
			"gc_cpu_fraction": m.GCCPUFraction,
		},
	}
}

// GetProfilingEndpoints returns available profiling endpoints
func GetProfilingEndpoints() []string {
	if !pprofEnabled {
		return []string{}
	}

	return []string{
		"/debug/pprof/",
		"/debug/pprof/goroutine",
		"/debug/pprof/heap",
		"/debug/pprof/threadcreate",
		"/debug/pprof/block",
		"/debug/pprof/mutex",
		"/debug/pprof/profile?seconds=30", // CPU profile
		"/debug/pprof/trace?seconds=30",   // Trace
	}
}

// PrintProfilingInstructions prints instructions for using profiling
func PrintProfilingInstructions(port string) {
	if !pprofEnabled {
		println("Profiling is not enabled. Set ACTOR_PPROF_ENABLE=1 or configure with ProfilingConfig.")
		return
	}

	if port == "" {
		port = defaultProfilingPort
	}

	baseURL := GetProfilingURL(port)
	println("Profiling is enabled!")
	println("")
	println("To collect profiles:")
	println("  CPU Profile (30s):    curl http://127.0.0.1:" + port + "/debug/pprof/profile?seconds=30 > cpu.prof")
	println("  Memory Profile:       curl http://127.0.0.1:" + port + "/debug/pprof/heap > heap.prof")
	println("  Goroutine Profile:    curl http://127.0.0.1:" + port + "/debug/pprof/goroutine > goroutine.prof")
	println("")
	println("To analyze profiles:")
	println("  go tool pprof cpu.prof")
	println("  go tool pprof heap.prof")
	println("")
	println("For interactive profiling, visit: " + baseURL)
}

// GetGoroutineCount returns the current goroutine count
func GetGoroutineCount() int {
	return runtime.NumGoroutine()
}

// GetMemStats returns memory statistics
func GetMemStats() runtime.MemStats {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats
}
