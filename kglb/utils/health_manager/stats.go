package health_manager

import (
	"dropbox/vortex2/v2stats"
)

// KGLB healthcheck counter.
// Tags:
// - setup: setup name
// - service: service name
// - host: actual host name being health checked
// - result: pass/fail
var healthCheckCounter = v2stats.MustDefineCounter("kglb/control_plane/healthcheck", "setup", "service", "host", "result")

var aliveRatioGauge = v2stats.MustDefineGauge("kglb/control_plane/alive_ratio", "setup", "service")
