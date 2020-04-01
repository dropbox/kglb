package data_plane

import (
	"net"

	kglb_pb "dropbox/proto/kglb"
)

type ResolverModule interface {
	// lookup (hostname resolution).
	Lookup(name string) (*HostnameCacheEntry, error)
	// reverse lookup (ip -> hostname resolution).
	ReverseLookup(ip net.IP) (string, error)
	// Get service name based on information available in IpvsService message.
	ServiceLookup(srv *kglb_pb.IpvsService) string
	// cluster name by hostname.
	Cluster(hostname string) string
}

type HostnameCacheEntry struct {
	IPv4 net.IP
	IPv6 net.IP
}
