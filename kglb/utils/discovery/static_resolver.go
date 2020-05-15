package discovery

import (
	"sync"

	"dropbox/dlog"
	"dropbox/vortex2/v2stats"
)

// Static resolver specific params.
type StaticResolverParams struct {
	// Resolver Id.
	Id string
	// Initial state.
	Hosts DiscoveryState

	// Used to report v2 stat
	SetupName   string
	ServiceName string
}

// Static Resolver implementation.
type StaticResolver struct {
	// resolver id.
	id string
	// current state of the resolver.
	state DiscoveryState

	// update channel.
	updateChan chan DiscoveryState
	closeOnce  sync.Once
	mutex      *sync.Mutex

	// v2 stats
	statResolverGauge v2stats.Gauge
}

func NewStaticResolver(params StaticResolverParams) (*StaticResolver, error) {
	var hostPorts []*HostPort
	for _, entry := range params.Hosts {
		hostPorts = append(
			hostPorts,
			&HostPort{Host: entry.Host, Port: entry.Port, Address: entry.Host, Enabled: entry.Enabled})
	}

	resolver := &StaticResolver{
		id:         params.Id,
		state:      DiscoveryState(hostPorts),
		mutex:      &sync.Mutex{},
		updateChan: make(chan DiscoveryState, 1),

		statResolverGauge: staticResolverGauge.Must(v2stats.KV{
			"setup":   params.SetupName,
			"service": params.ServiceName,
		}),
	}

	// put initial state into update channel since there will not be any auto
	// update based on discovery changes.
	resolver.updateChan <- resolver.state

	return resolver, nil
}

// Returns resolver id.
func (r *StaticResolver) GetId() string {
	return r.id
}

// Implements DiscoveryResolver interface
func (r *StaticResolver) GetState() DiscoveryState {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.state
}

// Updates state.
func (r *StaticResolver) Update(newState DiscoveryState) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	dlog.Infof("Update state of '%s' resolver: %v", r.id, newState)

	// 1. update internal state.
	r.state = newState

	// Update gauge
	r.statResolverGauge.Set(float64(len(newState)))

	// remove state from chan if any
	select {
	case <-r.updateChan:
	default:
	}

	// 2. notify about the change.
	r.updateChan <- r.state
}

// Returns update channel.
func (r *StaticResolver) Updates() <-chan DiscoveryState {
	return r.updateChan
}

// Implements DiscoveryResolver interface.
func (r *StaticResolver) Close() {
	dlog.Infof("Closing '%s' resolver", r.id)

	r.closeOnce.Do(func() {
		close(r.updateChan)
	})
}

// Check if the item discovers exactly the same things.
func (r *StaticResolver) Equal(item DiscoveryResolver) bool {
	if _, ok := item.(*StaticResolver); ok {
		thisState := r.GetState()
		itemState := item.GetState()
		if len(thisState) != len(itemState) {
			return false
		}

		if r.GetId() != item.GetId() {
			return false
		}

		for i, _ := range thisState {
			if !thisState[i].Equal(itemState[i]) {
				return false
			}
		}
		// states are exactly the same including order of the items.
		return true
	}

	return false
}

var _ DiscoveryResolver = &StaticResolver{}
