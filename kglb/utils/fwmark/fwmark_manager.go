package fwmark

import (
	"fmt"
	"sync"
)

type fwmarkMdata struct {
	fwmark   uint32
	refcount uint32
}

// Manager handles all fwmark allocation related routines.
type Manager struct {
	maxFwmark uint32
	base      uint32

	fwmarkLock  sync.RWMutex
	fwmarksPool []uint32
	allocated   map[string]*fwmarkMdata
}

// NewManager returns new Manager object w/ maximum specified
// unique ids for fwmarks started from base value
func NewManager(maxFwmark uint32, base uint32) *Manager {
	manager := &Manager{
		fwmarksPool: make([]uint32, 0, maxFwmark),
		allocated:   make(map[string]*fwmarkMdata),
		maxFwmark:   maxFwmark,
		base:        base,
	}
	for i := base; i < base+maxFwmark; i++ {
		manager.fwmarksPool = append(manager.fwmarksPool, i)
	}
	return manager
}

// AllocateFwmark returns next unused fwmark
func (m *Manager) AllocateFwmark(address string) (uint32, error) {
	m.fwmarkLock.Lock()
	defer m.fwmarkLock.Unlock()

	if v, exists := m.allocated[address]; exists {
		v.refcount++
		return v.fwmark, nil
	}

	if len(m.fwmarksPool) == 0 {
		return 0, fmt.Errorf("no more unused fwmarks")
	}
	fwmark := m.fwmarksPool[0]
	m.fwmarksPool = m.fwmarksPool[1:]
	m.allocated[address] = &fwmarkMdata{
		fwmark:   fwmark,
		refcount: 1,
	}
	return fwmark, nil
}

// GetAllocatedFwmark returns fwmark for address if it was already allocated
// (and hence wont bump refcount) or error if it was not allocated prior
func (m *Manager) GetAllocatedFwmark(address string) (uint32, error) {
	m.fwmarkLock.RLock()
	defer m.fwmarkLock.RUnlock()

	if v, exists := m.allocated[address]; exists {
		return v.fwmark, nil
	}
	return 0, fmt.Errorf("no fwmark allocated for address: %v", address)
}

// ReleaseFwmark returns unused fwmark into the pool
func (m *Manager) ReleaseFwmark(address string) error {
	m.fwmarkLock.Lock()
	defer m.fwmarkLock.Unlock()

	if v, exists := m.allocated[address]; exists {
		v.refcount--
		if v.refcount == 0 {
			m.fwmarksPool = append(m.fwmarksPool, v.fwmark)
			delete(m.allocated, address)
		}
		return nil
	}
	return fmt.Errorf("no allocated fwmark for address %v", address)
}
