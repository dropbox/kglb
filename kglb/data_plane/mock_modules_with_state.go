package data_plane

import (
	"fmt"
	"net"
	"sort"
	"sync"

	"dropbox/kglb/common"
	kglb_pb "dropbox/proto/kglb"
)

var _ BgpModule = NewMockBgpModuleWithState()
var _ BgpModule = NewEmptyBgpModuleWithState()
var _ IpvsModule = NewMockIpvsModuleWithState()
var _ AddressTableModule = NewMockAddressTableWithState()

// BGP unfunctional modules.
func NewEmptyBgpModuleWithState() BgpModule {
	return &MockBgpModule{
		InitFunc: func(asn uint32) error {
			return nil
		},
		AdvertiseFunc: func(bgpConfig *kglb_pb.BgpRouteAttributes) error {
			return notImplErr
		},
		WithdrawFunc: func(bgpConfig *kglb_pb.BgpRouteAttributes) error {
			return notImplErr
		},
		ListPathsFunc: func() ([]*kglb_pb.BgpRouteAttributes, error) {
			return []*kglb_pb.BgpRouteAttributes{}, nil
		},
		IsSessionEstablishedFunc: func() (bool, error) {
			return true, nil
		},
	}
}

// BGP modules maintaines fake state.
func NewMockBgpModuleWithState() BgpModule {
	bgpConfigsState := make([]*kglb_pb.BgpRouteAttributes, 0)

	return &MockBgpModule{
		InitFunc: func(asn uint32) error {
			return nil
		},
		AdvertiseFunc: func(bgpConfig *kglb_pb.BgpRouteAttributes) error {
			for _, cfg := range bgpConfigsState {
				if common.BgpRoutingAttributesComparable.Equal(cfg, bgpConfig) {
					return fmt.Errorf(
						"Bgp route already advertised: %+v",
						bgpConfig)
				}
			}
			bgpConfigsState = append(bgpConfigsState, bgpConfig)
			return nil
		},
		WithdrawFunc: func(bgpConfig *kglb_pb.BgpRouteAttributes) error {
			for i, cfg := range bgpConfigsState {
				if common.BgpRoutingAttributesComparable.Equal(cfg, bgpConfig) {
					bgpConfigsState = append(
						bgpConfigsState[:i],
						bgpConfigsState[i+1:]...)
					return nil
				}
			}
			return fmt.Errorf(
				"Bgp route not found: %+v",
				bgpConfig)
		},
		ListPathsFunc: func() ([]*kglb_pb.BgpRouteAttributes, error) {
			return bgpConfigsState, nil
		},
		IsSessionEstablishedFunc: func() (bool, error) {
			return true, nil
		},
	}
}

// Mock IPVS module with state
type mockIpvsData struct {
	service *kglb_pb.IpvsService
	reals   []*kglb_pb.UpstreamState
}

type MockIpvsModuleWithState struct {
	MockIpvsModule
	mu       sync.Mutex
	Services []*mockIpvsData
}

func NewMockIpvsModuleWithState() IpvsModule {
	m := &MockIpvsModuleWithState{}
	m.MockIpvsModule = MockIpvsModule{
		AddServiceFunc: func(service *kglb_pb.IpvsService) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.Services = append(
				m.Services,
				&mockIpvsData{
					service: service, reals: []*kglb_pb.UpstreamState{},
				})
			return nil
		},
		DeleteServiceFunc: func(service *kglb_pb.IpvsService) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			for i, configuredService := range m.Services {
				if common.IPVSServicesEqual(configuredService.service, service) {
					m.Services = append(m.Services[:i], m.Services[i+1:]...)
					return nil
				}
			}
			return fmt.Errorf("no service")
		},
		// Get list of existent ipvs Services.
		ListServicesFunc: func() ([]*kglb_pb.IpvsService, []*kglb_pb.Stats, error) {
			m.mu.Lock()
			defer m.mu.Unlock()

			var r []*kglb_pb.IpvsService
			var servicesStats []*kglb_pb.Stats
			for _, service := range m.Services {
				r = append(r, service.service)
				servicesStats = append(servicesStats, &kglb_pb.Stats{})
			}
			return r, servicesStats, nil
		},
		AddRealServerFunc: func(service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error {
			cService := m.findService(service)
			if cService == nil {
				return fmt.Errorf("no service")
			}
			cService.reals = append(cService.reals, dst)
			return nil
		},
		// Delete destination from specific ipvs service.
		DeleteRealServerFunc: func(service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error {
			cService := m.findService(service)
			if cService == nil {
				return fmt.Errorf("no service")
			}

			for i, real := range cService.reals {
				if common.UpstreamsEqual(real, dst) {
					cService.reals = append(cService.reals[:i], cService.reals[i+1:]...)
				}
			}
			return nil
		},
		// Update destination for specific ipvs service.
		UpdateRealServerFunc: func(service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error {
			cService := m.findService(service)
			if cService == nil {
				return fmt.Errorf("no service")
			}

			for _, real := range cService.reals {
				if common.UpstreamsEqual(real, dst) {
					real.Weight = dst.Weight
				}
			}
			return nil
		},
		// Get list of destinations of specific ipvs service
		GetRealServersFunc: func(
			service *kglb_pb.IpvsService) ([]*kglb_pb.UpstreamState, []*kglb_pb.Stats, error) {

			cService := m.findService(service)
			if cService == nil {
				return nil, nil, fmt.Errorf("no service")
			}

			realStats := make([]*kglb_pb.Stats, len(cService.reals))
			for i := range cService.reals {
				realStats[i] = &kglb_pb.Stats{}
			}
			return cService.reals, realStats, nil
		},
	}

	return m
}

func (m *MockIpvsModuleWithState) findService(
	service *kglb_pb.IpvsService) *mockIpvsData {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, configuredService := range m.Services {
		if common.IPVSServicesEqual(configuredService.service, service) {
			return configuredService
		}
	}
	return nil
}

func NewMockAddressTableWithState() AddressTableModule {
	addrMap := map[string][]net.IP{
		"lo": {net.ParseIP("127.0.0.1")},
	}

	return &MockAddressTableModule{
		AddFunc: func(addr net.IP, iface string) error {
			if ifaceState, ok := addrMap[iface]; !ok {
				return fmt.Errorf("wrong interface: %v", iface)
			} else {
				addrMap[iface] = append(ifaceState, addr)
			}
			return nil
		},
		DeleteFunc: func(addr net.IP, iface string) error {
			if ifaceState, ok := addrMap[iface]; !ok {
				return fmt.Errorf("wrong interface: %v", iface)
			} else {
				for i, existentAddr := range ifaceState {
					if existentAddr.Equal(addr) {
						addrMap[iface] = append(
							ifaceState[:i],
							ifaceState[i+1:]...)
						return nil
					}
				}
				return fmt.Errorf(
					"address doesn't exist: %v, iface: %v",
					addr.String(),
					iface)
			}
		},
		IsExistsFunc: func(addr net.IP, iface string) (bool, error) {
			if ifaceState, ok := addrMap[iface]; !ok {
				return false, fmt.Errorf("wrong interface: %v", iface)
			} else {
				for _, existentAddr := range ifaceState {
					if existentAddr.Equal(addr) {
						return true, nil
					}
				}
				return false, nil
			}
		},
		ListFunc: func(iface string) ([]net.IP, error) {
			if ifaceState, ok := addrMap[iface]; !ok {
				return nil, fmt.Errorf("wrong interface: %v", iface)
			} else {
				sort.Slice(ifaceState, func(i, j int) bool {
					return ifaceState[i].String() < ifaceState[j].String()
				})
				return ifaceState, nil
			}
		},
	}
}
