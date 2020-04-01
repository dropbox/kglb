package data_plane

import (
	"context"
	"net"
	"time"

	"godropbox/errors"

	kglb_pb "dropbox/proto/kglb"
)

var (
	// default resolution timeout.
	defaultResolutionTimeout = 500 * time.Millisecond
)

const (
	// value of nknown service_name tag in case of failed resolution.
	unknownService = "default"
	// value of hostname tag in case of failed resolution.
	unknownHostname = "unknown"
	// value of real_cluster tag in case of failed resolution.
	unknownRealCluster = "none"
)

type Resolver struct {
	resolver *net.Resolver
}

func NewResolver() (*Resolver, error) {
	return &Resolver{
		resolver: &net.Resolver{},
	}, nil
}

// lookup (hostname resolution).
func (r *Resolver) Lookup(name string) (*HostnameCacheEntry, error) {
	ctx, cancel := context.WithTimeout(
		context.TODO(),
		defaultResolutionTimeout)
	defer cancel()
	var result = &HostnameCacheEntry{}

	addrs, err := r.resolver.LookupIPAddr(ctx, name)
	if err != nil {
		return nil, err
	}

	if len(addrs) == 0 {
		return nil, errors.Newf(
			"fails to resolve %s name, reason: %v", name, err)
	}

	for _, addr := range addrs {
		if addr.IP.To4() != nil {
			result.IPv4 = addr.IP
		} else {
			result.IPv6 = addr.IP
		}
	}
	return result, nil
}

// cluster name by hostname
func (r *Resolver) Cluster(hostname string) string {
	if len(hostname) > 3 {
		return hostname[:3]
	}
	return unknownRealCluster
}

// reverse lookup (ip -> hostname resolution).
func (r *Resolver) ReverseLookup(ip net.IP) (string, error) {
	ctx, cancel := context.WithTimeout(
		context.TODO(),
		defaultResolutionTimeout)
	defer cancel()

	names, err := r.resolver.LookupAddr(ctx, ip.String())
	if err != nil {
		return unknownHostname, err
	}

	if len(names) == 0 {
		return unknownHostname, errors.Newf(
			"fails to resolve %s addr, reason: %v", ip.String(), err)
	}

	return names[0], nil
}

// Get service name by information available in IpvsService message.
func (r *Resolver) ServiceLookup(srv *kglb_pb.IpvsService) string {
	return unknownService
}
