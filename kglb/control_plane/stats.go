package control_plane

import (
	"dropbox/vortex2/v2stats"
)

// Current balancer state (on given setup/service)
// Tags:
// - state: current state of balancer [initial, available, no_upstreams, failsafe, shutdown]
// - setup: setup name
// - service: service name
var balancerStateGauge = v2stats.MustDefineGauge("kglb/control_plane/balancer_state", "setup", "service", "state")

// Route announcement gauge
// Tags:
// - state: current state of route announcement: [on/off]
// - setup: setup name
// - service: service name
var routeAnnouncementGauge = v2stats.MustDefineGauge("kglb/control_plane/route_announcement", "setup", "service", "state")

// Simple gauge of alive upstreams count.
// Tags:
// - setup: setup name
// - service: service name
// - alive: [true/false]
var upstreamsCountGauge = v2stats.MustDefineGauge("kglb/control_plane/upstreams_count", "setup", "service", "alive")

// KGLB availability gauge (gauge value indicates availabilit: 0/1)
// Tags:
// - initialized: [true/false] - indicates control plane initialization status
var availabilityGauge = v2stats.MustDefineGauge("kglb/control_plane/availability", "initialized")

// KGLB state hash gauge (gauge value being used to compare between different kglbs in same cluster)
// same value means they are consistent (see the same amount of up/down hosts)
var stateHashGauge = v2stats.MustDefineGauge("kglb/control_plane/state_hash", "entity", "setup")
