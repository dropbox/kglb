package main

import (
	"sync"

	"dropbox/kglb/common"
	kglb_pb "dropbox/proto/kglb"
)

// Mock Bgp module.
type NoOpBgpModule struct {
	mu    sync.Mutex
	state []*kglb_pb.BgpRouteAttributes
}

func (b *NoOpBgpModule) Init(asn uint32) error {
	return nil
}

func (b *NoOpBgpModule) Advertise(config *kglb_pb.BgpRouteAttributes) error {
	b.mu.Lock()
	b.mu.Unlock()

	b.state = append(b.state, config)
	return nil
}

func (b *NoOpBgpModule) Withdraw(config *kglb_pb.BgpRouteAttributes) error {
	b.mu.Lock()
	b.mu.Unlock()

	for i, cfg := range b.state {
		if common.BgpRoutingAttributesComparable.Equal(cfg, config) {
			b.state = append(b.state[:i], b.state[i+1:]...)
			break
		}
	}
	return nil
}

func (b *NoOpBgpModule) ListPaths() ([]*kglb_pb.BgpRouteAttributes, error) {
	b.mu.Lock()
	b.mu.Unlock()

	return b.state, nil
}

func (b *NoOpBgpModule) IsSessionEstablished() (bool, error) {
	return true, nil
}
