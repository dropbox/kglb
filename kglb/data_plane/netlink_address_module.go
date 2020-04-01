package data_plane

import (
	"net"
)

// Address list manipulations through netlink.
type AddressTableModule interface {
	// Add address to the interface.
	Add(addr net.IP, iface string) error
	// Remove address from the interface.
	Delete(addr net.IP, iface string) error
	// Check if the link had specific address.
	IsExists(addr net.IP, iface string) (bool, error)
	// List of all configured addresses for specific interface.
	List(iface string) ([]net.IP, error)
}
