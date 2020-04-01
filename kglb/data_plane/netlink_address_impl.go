package data_plane

import (
	"fmt"
	"net"
	"sort"

	"github.com/vishvananda/netlink"

	"godropbox/errors"
)

type netlinkFunc struct {
	// AddrAdd will add an IP address to a link device.
	AddrAdd func(link netlink.Link, addr *netlink.Addr) error
	// AddrDel will delete an IP address from a link device.
	AddrDel func(link netlink.Link, addr *netlink.Addr) error
	// / AddrList gets a list of IP addresses in the system.
	AddrList func(link netlink.Link, family int) ([]netlink.Addr, error)
	// ParseAddr parses the string representation of an address in the
	// form $ip/$netmask $label.
	ParseAddr func(s string) (*netlink.Addr, error)
	// Convert string representation of the link into internal structure.
	LinkByName func(name string) (netlink.Link, error)
}

// wrapper for github.com/vishvananda/netlink lib.
type NetlinkAddress struct {
	// netlink related funcs.
	nlApi *netlinkFunc
}

// using in tests to simplify mocking of low-level api.
func newNetLinkAddress(nlApi *netlinkFunc) (AddressTableModule, error) {
	return &NetlinkAddress{
		nlApi: nlApi,
	}, nil
}

func NewNetlinkAddress() (AddressTableModule, error) {
	// netlink related funcs.
	nlApi := &netlinkFunc{
		AddrAdd:    netlink.AddrAdd,
		AddrDel:    netlink.AddrDel,
		AddrList:   netlink.AddrList,
		ParseAddr:  netlink.ParseAddr,
		LinkByName: netlink.LinkByName,
	}

	return newNetLinkAddress(nlApi)
}

// Add the address to the interface.
func (m *NetlinkAddress) Add(addr net.IP, iface string) error {
	// 1. check if address already exists.
	if exist, err := m.IsExists(addr, iface); err != nil {
		return err
	} else if exist {
		return errors.Newf("address already exist: %v", addr.String())
	}

	// 2. Convert to required lib format.
	ntAddr, err := m.nlApi.ParseAddr(fmt.Sprintf("%s/32", addr.String()))
	if err != nil {
		return err
	}

	ntLink, err := m.nlApi.LinkByName(iface)
	if err != nil {
		return err
	}

	// 3. Adding the address.
	if err = m.nlApi.AddrAdd(ntLink, ntAddr); err != nil {
		return err
	}

	return nil
}

// Delete the address from the interface.
func (m *NetlinkAddress) Delete(addr net.IP, iface string) error {
	// 1. check if address exists, otherwise false.
	if exist, err := m.IsExists(addr, iface); err != nil {
		return err
	} else if !exist {
		return errors.Newf("address doesn't exist: %v", addr.String())
	}

	// 2. Convert to required lib format.
	ntAddr, err := m.nlApi.ParseAddr(fmt.Sprintf("%s/32", addr.String()))
	if err != nil {
		return err
	}

	ntLink, err := m.nlApi.LinkByName(iface)
	if err != nil {
		return err
	}

	// 3. Finally delete the address.
	if err = m.nlApi.AddrDel(ntLink, ntAddr); err != nil {
		return err
	}

	return nil
}

// Checks if the address belongs to the link, otherwise returns false.
func (m *NetlinkAddress) IsExists(addr net.IP, iface string) (bool, error) {
	addrs, err := m.List(iface)
	if err != nil {
		return false, err
	}

	for _, linkAddr := range addrs {
		if addr.Equal(linkAddr) {
			return true, nil
		}
	}

	return false, nil
}

// List of all configured addresses for specific interface.
func (m *NetlinkAddress) List(iface string) ([]net.IP, error) {
	ntLink, err := m.nlApi.LinkByName(iface)
	if err != nil {
		return nil, err
	}

	var addrList []net.IP

	for _, addrFamily := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		addrs, err := m.nlApi.AddrList(ntLink, addrFamily)
		if err != nil {
			return nil, errors.Wrap(err, "netlink.AddrList() fails: ")
		}

		for _, addr := range addrs {
			addrList = append(addrList, addr.IP)
		}

	}
	sort.Slice(addrList, func(i, j int) bool { return addrList[i].String() < addrList[j].String() })
	return addrList, nil
}
