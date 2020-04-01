package discovery

import (
	"dropbox/vortex2/v2stats"
)

// Indicates how many upstreams currently resolved.
// Tags:
// - setup: setup name
// - service: service name
var staticResolverGauge = v2stats.MustDefineGauge("kglb/control_plane/discovery/static_upstream_count", "setup", "service")
