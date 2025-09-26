package main

import (
	"log"
	"runtime"
	"time"
)

// Performance metrics helper
type Metrics struct {
	ActiveConnections int
	GamesSinceStart   int
	LastGameDuration  time.Duration
	MemoryUsage       runtime.MemStats
}

func (h *Hub) logMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	log.Printf("=== PERFORMANCE METRICS ===")
	log.Printf("Active connections: %d", len(h.clients))
	log.Printf("Active players: %d", len(h.gameState.Players))
	log.Printf("Memory Alloc: %d KB", bToKb(m.Alloc))
	log.Printf("Memory TotalAlloc: %d KB", bToKb(m.TotalAlloc))
	log.Printf("Memory Sys: %d KB", bToKb(m.Sys))
	log.Printf("NumGC: %d", m.NumGC)
	log.Printf("===========================")
}

func bToKb(b uint64) uint64 {
	return b / 1024
}

// Call this periodically to monitor performance
func (h *Hub) startMetricsLogger() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			h.logMetrics()
		}
	}()
}