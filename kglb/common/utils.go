package common

import (
	pb "dropbox/proto/kglb"
)

// Returns alive ratio of set of upstreams.
func AliveUpstreamsRatio(ups []*pb.UpstreamState) float32 {
	all := len(ups)

	alive := 0
	for _, u := range ups {
		if u.Weight != 0 {
			alive += 1
		}
	}

	if alive == 0 {
		return 0.0
	}

	return float32(alive) / float32(all)
}
