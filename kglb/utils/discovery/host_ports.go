package discovery

import (
	"net"
	"strconv"
)

// Resolver primitive.
type HostPort struct {
	// Hostname
	Host string
	// port number
	Port int
	// address or hostname to use for healthchecks (depends on DnsResolver config)
	Address string
	// flag which indicates if host is enabled for taking traffic or not
	Enabled bool
}

func NewHostPort(host string, port int, enabled bool) *HostPort {
	return &HostPort{
		Host:    host,
		Port:    port,
		Address: host,
		Enabled: enabled,
	}
}

// Compare HostPort items.
func (h *HostPort) Equal(item *HostPort) bool {
	if item != nil && h.Host == item.Host && h.Port == item.Port && h.Address == item.Address && h.Enabled == item.Enabled {
		return true
	}
	return false
}

func (h *HostPort) String() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
}

type DiscoveryState []*HostPort

func (h DiscoveryState) Len() int {
	return len(h)
}

// Returns true when DiscoveryState contains hostPort, otherwise false.
func (h DiscoveryState) Contains(hostPort *HostPort) bool {
	for _, entry := range h {
		if hostPort.Equal(entry) {
			return true
		}
	}

	return false
}

// Compare two DiscoveryState's
func (h DiscoveryState) Equal(state DiscoveryState) bool {
	if h.Len() != state.Len() {
		return false
	}

	for _, curEntry := range h {
		found := false
		for _, newEntry := range state {
			if curEntry.Equal(newEntry) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
