package data_plane

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"dropbox/dlog"
	"dropbox/exclog"
	"dropbox/kglb/common"
	kglb_pb "dropbox/proto/kglb"
	"godropbox/errors"
)

var (
	// value of nknown service_name tag in case of failed resolution.
	defaultService = "default"
	// value of hostname tag in case of failed resolution.
	defaultHostname = "default"
	// value of real_cluster tag in case of failed resolution.
	defaultRealCluster = "default"
)

// Implements Resolver interface based on cache approach.
type CacheResolver struct {
	mutex sync.RWMutex
	// cache map protected by mutex.
	// ip -> hostname map
	reverseCache map[string]string
	// hostname -> ip map
	hostnameCache map[string]*HostnameCacheEntry
	// vip:vport -> alias
	serviceCache map[string]string
}

func NewCacheResolver() (*CacheResolver, error) {
	return &CacheResolver{
		reverseCache:  make(map[string]string),
		hostnameCache: make(map[string]*HostnameCacheEntry),
		serviceCache:  make(map[string]string),
	}, nil
}

// lookup (hostname resolution).
func (r *CacheResolver) Lookup(name string) (*HostnameCacheEntry, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if val, ok := r.hostnameCache[name]; ok {
		return val, nil
	}

	err := errors.Newf("missed lookup cache: %v, %v", name, r.hostnameCache)
	exclog.Report(err, exclog.Noncritical, "")
	return nil, err
}

// cluster name for hostname.
func (r *CacheResolver) Cluster(hostname string) string {
	if len(hostname) == 0 || !strings.Contains(hostname, "-") {
		return defaultRealCluster
	}

	slices := strings.Split(hostname, "-")
	if len(slices) > 0 {
		return slices[0]
	}
	return defaultRealCluster
}

// reverse lookup (ip -> hostname resolution).
func (r *CacheResolver) ReverseLookup(ip net.IP) (string, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if val, ok := r.reverseCache[ip.String()]; ok {
		return val, nil
	}

	err := errors.Newf("missed reverse lookup cache: %v", ip.String())
	exclog.Report(err, exclog.Noncritical, "")

	return defaultHostname, nil
}

// vip:vport to human readable alias conversion.
func (r *CacheResolver) ServiceLookup(srv *kglb_pb.IpvsService) string {
	if srv == nil {
		return defaultService
	}

	// extract vip, port, fwmark
	key := r.keyByService(srv)

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if val, ok := r.serviceCache[key]; ok {
		return val
	}

	dlog.Errorf("missed service lookup cache: %v", key)

	return defaultService
}

// update map of internal naming/aliases to skip reverse-lookup since
// control plane sends all required information including hostname and aliases
// inside the config.
func (r *CacheResolver) UpdateCache(state *kglb_pb.DataPlaneState) {
	if state == nil {
		dlog.Info(
			"Skipping updating CacheResolver cache since state is empty")
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()

	cacheUpdated := false

	for _, service := range state.Balancers {
		alias := service.GetName()
		key := r.keyByService(service.GetLbService().GetIpvsService())
		if entry, ok := r.serviceCache[key]; !ok {
			dlog.Infof(
				"Adding service cache record: %s <-> %s",
				key,
				alias)
		} else if entry != alias {
			dlog.Infof(
				"Updating service cache record: %s: %s -> %s",
				key,
				entry,
				alias)
		}
		r.serviceCache[key] = alias

		// update real server hostname <-> ip caches.
		for _, upstreamState := range service.Upstreams {
			hostname := upstreamState.GetHostname()
			if len(hostname) == 0 {
				exclog.Report(
					errors.Newf("missed hostname field: %+v", upstreamState),
					exclog.Noncritical, "")
				continue
			}
			address := common.KglbAddrToNetIp(upstreamState.GetAddress())
			if len(address) == 0 {
				exclog.Report(
					errors.Newf("missed address field: %+v", upstreamState),
					exclog.Noncritical, "")
				continue
			}

			if _, ok := r.hostnameCache[hostname]; !ok {
				r.hostnameCache[hostname] = &HostnameCacheEntry{}
			}

			if address.To4() != nil && !r.hostnameCache[hostname].IPv4.Equal(address) {
				dlog.Infof(
					"Adding hostname v4 cache record: %s <-> %s",
					hostname,
					address.String())
				r.hostnameCache[hostname].IPv4 = address
				cacheUpdated = true
			} else if address.To4() == nil && !r.hostnameCache[hostname].IPv6.Equal(address) {
				dlog.Infof(
					"Adding hostname v6 cache record: %s <-> %s",
					hostname,
					address.String())
				r.hostnameCache[hostname].IPv6 = address
				cacheUpdated = true
			}

			if cachedHostname, ok := r.reverseCache[address.String()]; !ok || cachedHostname != hostname {
				dlog.Infof(
					"Adding reverse cache record: %s <-> %s",
					address.String(),
					hostname)
				r.reverseCache[address.String()] = hostname
				cacheUpdated = true
			}
		}
	}

	if cacheUpdated {
		// log size of caches to control sizes in alerts since update adds values
		// without removing old keys.
		dlog.Infof(
			"service_cache_size: %d, hostname_cache_size: %d, reverse_cache_size: %d",
			len(r.serviceCache),
			len(r.hostnameCache),
			len(r.reverseCache))
	}
}

// generate key based on IpvsService settings to use it as a key for serviceCache map.
func (r *CacheResolver) keyByService(srv *kglb_pb.IpvsService) string {
	// extract vip, port, fwmark
	switch attr := srv.Attributes.(type) {
	case *kglb_pb.IpvsService_TcpAttributes:
		return fmt.Sprintf(
			"tcp-%s:%d",
			common.KglbAddrToNetIp(attr.TcpAttributes.GetAddress()),
			attr.TcpAttributes.Port)
	case *kglb_pb.IpvsService_UdpAttributes:
		return fmt.Sprintf(
			"udp-%s:%d",
			common.KglbAddrToNetIp(attr.UdpAttributes.GetAddress()),
			attr.UdpAttributes.Port)
	case *kglb_pb.IpvsService_FwmarkAttributes:
		return fmt.Sprintf(
			"fwmark-%d",
			attr.FwmarkAttributes.GetFwmark())
	default:
		err := errors.Newf(
			"service lookup fails because of unknown attributes type: %s, service: %+v",
			attr,
			srv)
		exclog.Report(err, exclog.Noncritical, "")
	}
	return defaultService
}
