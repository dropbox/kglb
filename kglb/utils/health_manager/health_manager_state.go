package health_manager

import (
	"fmt"

	"dropbox/kglb/utils/discovery"
)

type HealthManagerEntry struct {
	HostPort *discovery.HostPort
	Status   *healthStatusEntry
	Enabled  bool
}

// Compares two HealthManagerEntry entries and returns true when both are equal,
// otherwise false.
func (h *HealthManagerEntry) Equal(entry *HealthManagerEntry) bool {
	return h.HostPort.Equal(entry.HostPort) && h.Status.Equal(entry.Status)
}

// Returns new HealthManagerEntry instance.
func NewHealthManagerEntry(
	initialHealthyState bool,
	host string,
	port int) HealthManagerEntry {

	return HealthManagerEntry{
		HostPort: discovery.NewHostPort(host, port, true),
		Status:   NewHealthStatusEntry(initialHealthyState),
	}
}

type HealthManagerState []HealthManagerEntry

// Returns true when HealthManagerState contains provided hostPort entry,
// otherwise false.
func (h HealthManagerState) Contains(hostPort *discovery.HostPort) bool {
	entry := h.GetEntry(hostPort)
	return entry != nil
}

// Returns reference to the entry for provided HostPort when state has it,
// otherwise nil.
func (h HealthManagerState) GetEntry(
	hostPort *discovery.HostPort) *HealthManagerEntry {

	for _, entry := range h {
		if hostPort.Equal(entry.HostPort) {
			return &entry
		}
	}

	return nil
}

// Returns true when at least one entry is healthy.
func (h HealthManagerState) IsHealthy() bool {
	for _, entry := range h {
		if entry.Status.IsHealthy() {
			return true
		}
	}

	return false
}

// Convert all entries into the string.
func (h HealthManagerState) String() string {
	out := "["
	prefix := ""
	for i, entry := range h {
		if i != 0 {
			prefix = ", "
		}
		out += fmt.Sprintf(
			"%s%s/%v",
			prefix,
			entry.HostPort.String(),
			entry.Status.IsHealthy())
	}
	out += "]"
	return out
}

// Returns new copy of HealthManagerState (it's not thread-safe call).
func (h HealthManagerState) Clone() HealthManagerState {
	newState := make(HealthManagerState, len(h))

	for i, entry := range h {
		newState[i] = HealthManagerEntry{
			HostPort: discovery.NewHostPort(
				entry.HostPort.Host,
				entry.HostPort.Port,
				entry.HostPort.Enabled),
			Status: &healthStatusEntry{
				isHealthy:   entry.Status.isHealthy,
				healthCount: entry.Status.healthCount,
			},
		}
	}

	return newState
}

// Returns true when all HostPort entries equal to provided DiscoveryState,
// otherwise false.
func (h HealthManagerState) Equal(state HealthManagerState) bool {
	if len(h) != len(state) {
		return false
	}

	// TODO(dkopytkov): improve comparison since it may check the same items
	// twice.
	for _, entry := range h {
		if !state.Contains(entry.HostPort) {
			return false
		}
	}

	for _, entry := range state {
		if !h.Contains(entry.HostPort) {
			return false
		}
	}

	return true
}
