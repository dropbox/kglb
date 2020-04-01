package data_plane

import (
	"net"

	kglb_pb "dropbox/proto/kglb"
)

// set of fake modules with no functionality inside, but to simplify
// overriding required functions.

// Bgp
type MockBgpModule struct {
	InitFunc                 func(asn uint32) error
	AdvertiseFunc            func(bgpConfig *kglb_pb.BgpRouteAttributes) error
	WithdrawFunc             func(bgpConfig *kglb_pb.BgpRouteAttributes) error
	ListPathsFunc            func() ([]*kglb_pb.BgpRouteAttributes, error)
	IsSessionEstablishedFunc func() (bool, error)
}

func (m *MockBgpModule) Init(asn uint32) error {
	if m.AdvertiseFunc == nil {
		return notImplErr
	}

	return m.InitFunc(asn)
}

func (m *MockBgpModule) Advertise(bgpConfig *kglb_pb.BgpRouteAttributes) error {
	if m.AdvertiseFunc == nil {
		return notImplErr
	}

	return m.AdvertiseFunc(bgpConfig)
}

func (m *MockBgpModule) Withdraw(bgpConfig *kglb_pb.BgpRouteAttributes) error {
	if m.WithdrawFunc == nil {
		return notImplErr
	}

	return m.WithdrawFunc(bgpConfig)
}

func (m *MockBgpModule) ListPaths() ([]*kglb_pb.BgpRouteAttributes, error) {
	if m.WithdrawFunc == nil {
		return nil, notImplErr
	}

	return m.ListPathsFunc()
}

func (m *MockBgpModule) IsSessionEstablished() (bool, error) {
	if m.IsSessionEstablishedFunc == nil {
		return false, notImplErr
	}
	return m.IsSessionEstablishedFunc()
}

// AddressTableModule.
type MockAddressTableModule struct {
	AddFunc      func(addr net.IP, iface string) error
	DeleteFunc   func(addr net.IP, iface string) error
	IsExistsFunc func(addr net.IP, iface string) (bool, error)
	ListFunc     func(iface string) ([]net.IP, error)
}

func (m *MockAddressTableModule) Add(addr net.IP, iface string) error {
	if m.AddFunc == nil {
		return notImplErr
	}

	return m.AddFunc(addr, iface)
}

func (m *MockAddressTableModule) Delete(addr net.IP, iface string) error {
	if m.DeleteFunc == nil {
		return notImplErr
	}

	return m.DeleteFunc(addr, iface)
}

func (m *MockAddressTableModule) IsExists(addr net.IP, iface string) (bool, error) {
	if m.IsExistsFunc == nil {
		return false, notImplErr
	}

	return m.IsExistsFunc(addr, iface)
}

func (m *MockAddressTableModule) List(iface string) ([]net.IP, error) {

	if m.ListFunc == nil {
		return nil, notImplErr
	}

	return m.ListFunc(iface)
}

type MockIpvsModule struct {
	AddServiceFunc func(service *kglb_pb.IpvsService) error
	// Delete ipvs service.
	DeleteServiceFunc func(service *kglb_pb.IpvsService) error
	// Get list of existent ipvs Services.
	ListServicesFunc func() ([]*kglb_pb.IpvsService, []*kglb_pb.Stats, error)

	// Add destination to the specific ipvs service.
	AddRealServerFunc func(service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error
	// Delete destination from specific ipvs service.
	DeleteRealServerFunc func(service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error
	// Update destination for specific ipvs service.
	UpdateRealServerFunc func(service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error
	// Get list of destinations of specific ipvs service
	GetRealServersFunc func(
		service *kglb_pb.IpvsService) ([]*kglb_pb.UpstreamState, []*kglb_pb.Stats, error)
}

func (m *MockIpvsModule) AddService(service *kglb_pb.IpvsService) error {
	if m.AddServiceFunc != nil {
		return m.AddServiceFunc(service)
	}
	return notImplErr
}

// Delete ipvs service.
func (m *MockIpvsModule) DeleteService(service *kglb_pb.IpvsService) error {
	if m.DeleteServiceFunc != nil {
		return m.DeleteServiceFunc(service)
	}
	return notImplErr
}

// Get list of existent ipvs Services.
func (m *MockIpvsModule) ListServices() ([]*kglb_pb.IpvsService, []*kglb_pb.Stats, error) {
	if m.ListServicesFunc != nil {
		return m.ListServicesFunc()
	}
	return nil, nil, notImplErr
}

// Add destination to the specific ipvs service.
func (m *MockIpvsModule) AddRealServers(
	service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error {
	if m.AddRealServerFunc != nil {
		for _, dst := range dsts {
			if err := m.AddRealServerFunc(service, dst); err != nil {
				return err
			}
		}
		return nil
	}
	return notImplErr
}

// Delete destination from specific ipvs service.
func (m *MockIpvsModule) DeleteRealServers(
	service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error {
	if m.DeleteRealServerFunc != nil {
		for _, dst := range dsts {
			if err := m.DeleteRealServerFunc(service, dst); err != nil {
				return err
			}
		}
		return nil
	}
	return notImplErr
}

// Update destination for specific ipvs service.
func (m *MockIpvsModule) UpdateRealServers(
	service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error {
	if m.UpdateRealServerFunc != nil {
		for _, dst := range dsts {
			if err := m.UpdateRealServerFunc(service, dst); err != nil {
				return err
			}
		}
		return nil
	}
	return notImplErr
}

// Get list of destinations of specific ipvs service
func (m *MockIpvsModule) GetRealServers(
	service *kglb_pb.IpvsService) ([]*kglb_pb.UpstreamState, []*kglb_pb.Stats, error) {

	if m.GetRealServersFunc != nil {
		return m.GetRealServersFunc(service)
	}
	return nil, nil, notImplErr
}

var _ IpvsModule = &MockIpvsModule{}
