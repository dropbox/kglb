package control_plane

import (
	"time"

	kglb_pb "dropbox/proto/kglb"
)

const (
	// how often do we send state to data plane
	DefaultSendInterval = 30 * time.Second

	// how long we can wait for data plane
	DefaultSendTimeout = 60 * time.Second
)

type DataPlaneClient interface {
	Set(state *kglb_pb.DataPlaneState) error
}
