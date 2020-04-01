package data_plane

import (
	"dropbox/vortex2/v2stats"
)

// KGLB Dataplane manager state (defined as public since it is being used by dbx_data_plane)
// Tags:
// - state: current state [initialized, init_failed, available, set_state_failed, shutdown_failed]
var ManagerStateGauge = v2stats.MustDefineGauge("kglb/data_plane/manager_state", "state")

// Time since the last successful state change in seconds (since startup)
var ManagerStateAgeSec = v2stats.MustDefineGauge("kglb/data_plane/manager_state_age_sec")

// KGLB data plane adds IP address to lo interface
// Here is the gauge to track this state
// Tags:
// - address: Interface / IP address pair which is being added, e.g. "lo/1.2.3.4"
// - state: [alive, add_failed, delete_failed]
var linkAddressGauge = v2stats.MustDefineGauge("kglb/data_plane/link_address", "address", "state")

// Per Service stats //
//
// bytes received / sent
// Tags:
// - name: service name
// - direction: [in, out]
var serviceBytesCounter = v2stats.MustDefineCounter("kglb/data_plane/service_bytes", "name", "direction")

// Packets received / sent
// Tags:
// - name: service name
// - direction: [in, out]
var servicePacketsCounter = v2stats.MustDefineCounter("kglb/data_plane/service_packets", "name", "direction")

// Per Upstream Stats //
//
// Bytes received / sent
// Tags:
// - name: upstream name
// - direction: [in, out]
var upstreamBytesCounter = v2stats.MustDefineCounter("kglb/data_plane/upstream_bytes", "name", "direction")

// Packets received / sent per service
// Tags:
// - name: upstream name
// - direction: [in, out]
var upstreamPacketsCounter = v2stats.MustDefineCounter("kglb/data_plane/upstream_packets", "name", "direction")

// BGP
//
// BGP session state.
// Tags:
// - state [established, not_established]
var bgpSessionStateGauge = v2stats.MustDefineGauge("kglb/data_plane/bgp_session", "state")

// BGP routes state.
// Tags:
// - route - advertised IP CIDR, e.g. 162.125.248.1/32
var bgpRouteGauge = v2stats.MustDefineGauge("kglb/data_plane/bgp_route", "route")
