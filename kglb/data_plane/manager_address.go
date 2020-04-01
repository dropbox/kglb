package data_plane

import (
	"dropbox/exclog"
	"dropbox/kglb/common"
	kglb_pb "dropbox/proto/kglb"
	"dropbox/vortex2/v2stats"
	"godropbox/errors"
)

type AddressManagerParams struct {
	// netlink/addresses.
	AddressTable AddressTableModule
}

type AddressManager struct {
	params *AddressManagerParams
	// map of added addresses to maintain link state.
	state map[string]*kglb_pb.LinkAddress
}

func NewAddressManager(
	params AddressManagerParams) (*AddressManager, error) {

	return &AddressManager{
		params: &params,
		state:  make(map[string]*kglb_pb.LinkAddress),
	}, nil
}

func (m *AddressManager) AddAddresses(balancers []*kglb_pb.LinkAddress) error {
	for _, balancer := range balancers {
		err := m.AddAddress(balancer)
		if err != nil {
			return err
		}
	}
	return nil
}

// Add IP address to the loopback for the balancer if it's needed.
func (m *AddressManager) AddAddress(linkAddress *kglb_pb.LinkAddress) (err error) {
	address := linkAddress.GetAddress()
	serviceAddr := common.KglbAddrToNetIp(address)

	defer func() {
		if err != nil {
			gauge, err2 := linkAddressGauge.V(v2stats.KV{
				"address": serviceAddr.String(),
				"state":   "add_failed",
			})
			if err2 == nil {
				gauge.Set(1)
			} else {
				exclog.Report(errors.Wrap(err2,
					"unable to instantiate linkAddressGauge"), exclog.Critical, "")
			}
		}
	}()

	iface := linkAddress.GetLinkName()
	exists, err := m.params.AddressTable.IsExists(serviceAddr, iface)
	if err != nil {
		return errors.Wrapf(
			err,
			"failed to check if address is configured: %v: ",
			serviceAddr)
	}

	if !exists {
		err := m.params.AddressTable.Add(serviceAddr, iface)
		if err != nil {
			return errors.Wrapf(
				err,
				"failed to add VIP address to interface: %v",
				serviceAddr)
		}
	}

	gauge, err2 := linkAddressGauge.V(v2stats.KV{
		"address": serviceAddr.String(),
		"state":   "alive",
	})
	if err2 == nil {
		gauge.Set(1)
	} else {
		exclog.Report(errors.Wrap(err2,
			"unable to instantiate linkAddressGauge"), exclog.Critical, "")
	}

	// updating internal map to track added addresses.
	m.state[linkAddress.String()] = linkAddress

	return nil
}

func (m *AddressManager) DeleteAddresses(addresses []*kglb_pb.LinkAddress) error {
	for _, address := range addresses {
		err := m.DeleteAddress(address)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *AddressManager) DeleteAddress(linkAddress *kglb_pb.LinkAddress) (err error) {
	address := linkAddress.GetAddress()
	serviceAddr := common.KglbAddrToNetIp(address)

	defer func() {
		if err != nil {
			gauge, err2 := linkAddressGauge.V(v2stats.KV{
				"address": serviceAddr.String(),
				"state":   "delete_failed",
			})
			if err2 == nil {
				gauge.Set(1)
			} else {
				exclog.Report(errors.Wrap(err2,
					"unable to instantiate linkAddressGauge"), exclog.Critical, "")
			}
		}
	}()

	iface := linkAddress.GetLinkName()

	exists, err := m.params.AddressTable.IsExists(serviceAddr, iface)
	if err != nil {
		return errors.Wrapf(
			err,
			"failed to check if address is configured: %v: ",
			serviceAddr)
	}
	if exists {
		err := m.params.AddressTable.Delete(serviceAddr, iface)
		if err != nil {
			return errors.Wrapf(
				err,
				"failed to delete VIP address to interface: %v",
				serviceAddr)
		}
	}

	gauge, err2 := linkAddressGauge.V(v2stats.KV{
		"address": serviceAddr.String(),
		"state":   "alive",
	})
	if err2 == nil {
		gauge.Set(0)
	} else {
		exclog.Report(errors.Wrap(err2,
			"unable to instantiate linkAddressGauge"), exclog.Critical, "")
	}

	// updating internal map to track added addresses.
	if _, ok := m.state[linkAddress.String()]; ok {
		delete(m.state, linkAddress.String())
	}

	return nil
}

// deadcode: will be in use to dump addresses.
func (m *AddressManager) ListAddresses(iface string) ([]*kglb_pb.LinkAddress, error) {
	addresses, err := m.params.AddressTable.List(iface)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"failed to check if address is configured:",
		)
	}
	result := make([]*kglb_pb.LinkAddress, len(addresses))
	for i, address := range addresses {
		result[i] = &kglb_pb.LinkAddress{LinkName: iface, Address: common.NetIpToKglbAddr(address)}
	}
	return result, nil
}

// returns list of addresses added through AddressManager API and which are still
// active (not deleted).
func (m *AddressManager) State() ([]*kglb_pb.LinkAddress, error) {
	var state []*kglb_pb.LinkAddress
	for _, link := range m.state {
		// double checking that address is still existent.
		serviceAddr := common.KglbAddrToNetIp(link.GetAddress())
		exists, err := m.params.AddressTable.IsExists(serviceAddr, link.GetLinkName())
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"failed to check if address is configured: %+v: ",
				link)
		}

		gauge, err := linkAddressGauge.V(v2stats.KV{
			"address": serviceAddr.String(),
			"state":   "alive",
		})
		if err != nil {
			exclog.Report(errors.Wrap(err,
				"unable to instantiate linkAddressGauge"), exclog.Critical, "")
		}
		if exists {
			gauge.Set(1)
			state = append(state, link)
		} else {
			gauge.Clear()
		}
	}

	return state, nil
}
