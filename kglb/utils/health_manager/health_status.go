package health_manager

import (
	"sync"
)

type healthStatusEntry struct {
	// Current status.
	isHealthy bool

	// positive = # of consecutive successful health checks
	// negative = # of consecutive failed health checks
	healthCount int

	// Mutex to protect fields of the struct.
	mutex sync.Mutex
}

// Returns new and initialized entry of healthStatusEntry.
func NewHealthStatusEntry(initialHealthyState bool) *healthStatusEntry {
	return &healthStatusEntry{
		isHealthy: initialHealthyState,
	}
}

// Getter to access internal isHealthy field. Returns true when current status
// of the entry is healthy, otherwise false.
func (h *healthStatusEntry) IsHealthy() bool {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.isHealthy
}

// Updates status of the entry based on latest health check results and returns
// true when status has been changed during UpdateEntry call.
func (h *healthStatusEntry) UpdateHealthCheckStatus(
	healthCheckPassed bool,
	riseCount int,
	fallCount int) bool {

	h.mutex.Lock()
	defer h.mutex.Unlock()

	if healthCheckPassed {
		if h.healthCount < 0 { // previous health check(s) failed
			h.healthCount = 1
		} else {
			h.healthCount++
		}

		if !h.isHealthy &&
			h.healthCount >= riseCount {

			h.isHealthy = true
			return true
		}

		return false
	}

	if h.healthCount > 0 { // previous health check(s) passed
		h.healthCount = -1
	} else {
		h.healthCount--
	}

	// NOTE: -entry.healthCount because we use negative value
	// to track failed health checks.
	if h.isHealthy &&
		-h.healthCount >= fallCount {

		h.isHealthy = false
		return true
	}

	return false
}

// Comparese two healthStatusEntry entries and returns true when both are
// identical (including healthy status and internal counter).
func (h *healthStatusEntry) Equal(entry *healthStatusEntry) bool {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.healthCount == entry.healthCount && h.isHealthy == entry.isHealthy
}
