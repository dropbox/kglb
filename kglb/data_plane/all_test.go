package data_plane

import (
	"testing"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	TestingT(t)
}

func GetMockModules(modules *ManagerModules) (*ManagerModules, error) {

	if modules == nil {
		modules = &ManagerModules{}
	}
	if modules.AddressTable == nil {
		modules.AddressTable = NewMockAddressTableWithState()
	}
	if modules.Ipvs == nil {
		modules.Ipvs = NewMockIpvsModuleWithState()
	}
	if modules.Resolver == nil {
		resolver, err := NewCacheResolver()
		if err != nil {
			return nil, err
		}
		modules.Resolver = resolver
	}
	if modules.Bgp == nil {
		modules.Bgp = NewMockBgpModuleWithState()
	}

	return modules, nil
}
