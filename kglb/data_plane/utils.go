package data_plane

import (
	"fmt"
	"strings"

	"dropbox/kglb/common"
	kglb_pb "dropbox/proto/kglb"
)

func diffMetric(new, old uint64) float64 {
	if new < old {
		return float64(new)
	}
	return float64(new - old)
}

func getBgpRoutePrefix(route *kglb_pb.BgpRouteAttributes) string {
	return fmt.Sprintf("%s/%d",
		common.KglbAddrToNetIp(route.GetPrefix()),
		route.GetPrefixlen())
}

func getBgpRouteName(route *kglb_pb.BgpRouteAttributes) string {
	return strings.Replace(
		strings.Replace(getBgpRoutePrefix(route), ":", ".", -1),
		" ", "_", -1)
}

// returns either Hostname or IP address.
func getUpstreamHostname(dst *kglb_pb.UpstreamState) string {
	if len(dst.Hostname) > 0 {
		return dst.Hostname
	}

	// TODO(belyalov): it used to be reverse hostname resolution here
	// however it seems to be redundant (just for stats!)
	// If we found that decent amount of upstreams have no hostname
	// proper fix would be to resolve hostname when creating (discovering?)
	// upstream.

	// if no hostname present - return IP
	return fmt.Sprintf("%v", common.KglbAddrToNetIp(dst.Address))
}
