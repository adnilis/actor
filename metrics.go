package actor

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics represents performance metric collection
type Metrics struct {
	MessagesSent      atomic.Uint64
	MessagesReceived  atomic.Uint64
	ActorCreations    atomic.Uint64
	ActorTerminations atomic.Uint64
	RegistryLookups   atomic.Uint64
	LockAcquisitions  atomic.Uint64

	// Latency measurements (in nanoseconds)
	TotalLatency atomic.Uint64
	LatencyCount atomic.Uint64

	// Start time for rate calculations
	StartTime atomic.Int64
}

// Global metrics instance
var (
	globalMetrics     *Metrics
	globalMetricsOnce sync.Once
	metricsEnabled    atomic.Bool
)

// EnableMetrics enables metrics collection
func EnableMetrics() {
	globalMetricsOnce.Do(func() {
		globalMetrics = &Metrics{
			StartTime: atomic.Int64{},
		}
		globalMetrics.StartTime.Store(time.Now().UnixNano())
	})
	metricsEnabled.Store(true)
}

// DisableMetrics disables metrics collection
func DisableMetrics() {
	metricsEnabled.Store(false)
}

// IsMetricsEnabled returns true if metrics collection is enabled
func IsMetricsEnabled() bool {
	return metricsEnabled.Load()
}

// GetMetrics returns the current metrics
func GetMetrics() *Metrics {
	if !metricsEnabled.Load() {
		return &Metrics{}
	}

	return globalMetrics
}

// ResetMetrics resets all metrics to zero
func ResetMetrics() {
	if globalMetrics == nil {
		return
	}

	globalMetrics.MessagesSent.Store(0)
	globalMetrics.MessagesReceived.Store(0)
	globalMetrics.ActorCreations.Store(0)
	globalMetrics.ActorTerminations.Store(0)
	globalMetrics.RegistryLookups.Store(0)
	globalMetrics.LockAcquisitions.Store(0)
	globalMetrics.TotalLatency.Store(0)
	globalMetrics.LatencyCount.Store(0)
	globalMetrics.StartTime.Store(time.Now().UnixNano())
}

// RecordMessageSent records a sent message
func RecordMessageSent() {
	if metricsEnabled.Load() && globalMetrics != nil {
		globalMetrics.MessagesSent.Add(1)
	}
}

// RecordMessageReceived records a received message
func RecordMessageReceived() {
	if metricsEnabled.Load() && globalMetrics != nil {
		globalMetrics.MessagesReceived.Add(1)
	}
}

// RecordActorCreation records an actor creation
func RecordActorCreation() {
	if metricsEnabled.Load() && globalMetrics != nil {
		globalMetrics.ActorCreations.Add(1)
	}
}

// RecordActorTermination records an actor termination
func RecordActorTermination() {
	if metricsEnabled.Load() && globalMetrics != nil {
		globalMetrics.ActorTerminations.Add(1)
	}
}

// RecordRegistryLookup records a registry lookup
func RecordRegistryLookup() {
	if metricsEnabled.Load() && globalMetrics != nil {
		globalMetrics.RegistryLookups.Add(1)
	}
}

// RecordLatency records a latency value in nanoseconds
func RecordLatency(latency int64) {
	if metricsEnabled.Load() && globalMetrics != nil {
		globalMetrics.TotalLatency.Add(uint64(latency))
		globalMetrics.LatencyCount.Add(1)
	}
}

// MetricsSnapshot represents a snapshot of current metrics
type MetricsSnapshot struct {
	MessagesPerSec     float64
	MessagesReceived   uint64
	MessagesSent       uint64
	ActiveActors       uint64
	ActorCreations     uint64
	RegistryLookupsSec float64
	AvgLatency         float64
	Uptime             time.Duration
}

// GetMetricsSnapshot returns a snapshot of current metrics with calculated rates
func GetMetricsSnapshot() MetricsSnapshot {
	if !metricsEnabled.Load() || globalMetrics == nil {
		return MetricsSnapshot{}
	}

	snapshot := MetricsSnapshot{
		MessagesReceived:   globalMetrics.MessagesReceived.Load(),
		MessagesSent:       globalMetrics.MessagesSent.Load(),
		ActorCreations:     globalMetrics.ActorCreations.Load(),
		RegistryLookupsSec: 0,
		AvgLatency:         0,
	}

	// Calculate uptime
	startTime := globalMetrics.StartTime.Load()
	if startTime > 0 {
		snapshot.Uptime = time.Duration(time.Now().UnixNano() - startTime)
	}

	// Calculate rates
	if snapshot.Uptime.Seconds() > 0 {
		snapshot.MessagesPerSec = float64(snapshot.MessagesReceived) / snapshot.Uptime.Seconds()
		snapshot.RegistryLookupsSec = float64(globalMetrics.RegistryLookups.Load()) / snapshot.Uptime.Seconds()
	}

	// Calculate average latency
	latencyCount := globalMetrics.LatencyCount.Load()
	if latencyCount > 0 {
		snapshot.AvgLatency = float64(globalMetrics.TotalLatency.Load()) / float64(latencyCount)
	}

	// Calculate active actors
	snapshot.ActiveActors = globalMetrics.ActorCreations.Load() - globalMetrics.ActorTerminations.Load()

	return snapshot
}

// PrintMetrics prints the current metrics to stdout
func PrintMetrics() {
	snapshot := GetMetricsSnapshot()

	println("=== Performance Metrics ===")
	println("Uptime:", snapshot.Uptime.String())
	println("Messages Received:", snapshot.MessagesReceived)
	println("Messages Sent:", snapshot.MessagesSent)
	println("Messages/sec:", snapshot.MessagesPerSec)
	println("Actor Creations:", snapshot.ActorCreations)
	println("Active Actors:", snapshot.ActiveActors)
	println("Registry Lookups/sec:", snapshot.RegistryLookupsSec)
	if snapshot.AvgLatency > 0 {
		println("Avg Latency:", snapshot.AvgLatency, "ns")
	}
	println("==========================")
}
