package main

import (
	"context"
	"time"

	"github.com/golang/glog"

	"dropbox/kglb/control_plane"
	"dropbox/kglb/data_plane"
	"dropbox/kglb/utils/dns_resolver"
	"dropbox/kglb/utils/fwmark"
	kglb_pb "dropbox/proto/kglb"
)

const (
	defaultMaxFwmark  = 5000
	defaultFwmarkBase = 10000
)

var (
	// dns resolution timeout.
	maxDnsResolveTime = 1 * time.Minute

	// how often do we send state to data plane.
	DefaultSendInterval = 30 * time.Second

	// emitting interval of ipvs and realserver stats.
	statsEmitInterval = 10 * time.Second
)

type Service struct {
	controlPlaneMng *control_plane.ControlPlaneServicer
	dataPlaneMng    *data_plane.Manager
}

func NewService(ctx context.Context, configPath string) (*Service, error) {
	s := &Service{}
	if err := s.initModules(ctx, configPath); err != nil {
		return nil, err
	}

	// stats emitter loop
	go s.statsLoop(ctx)

	return s, nil
}

// Apply data plane state.
func (s *Service) Set(state *kglb_pb.DataPlaneState) error {
	err := s.dataPlaneMng.SetState(state)
	if err != nil {
		glog.Errorf("fails to apply data plane state: %v", err)
	}

	return err
}

// Initialize all required modules and control/data planes.
func (s *Service) initModules(ctx context.Context, configPath string) error {
	var err error

	// initializing data plane related modules.
	dpModules := data_plane.ManagerModules{
		Bgp: &NoOpBgpModule{},
	}

	cacheResolver, err := data_plane.NewCacheResolver()
	if err != nil {
		return err
	}

	dpModules.Resolver = cacheResolver

	if dpModules.Ipvs, err = data_plane.NewIpvsMqLiang(cacheResolver); err != nil {
		return err
	}

	if dpModules.AddressTable, err = data_plane.NewNetlinkAddress(); err != nil {
		return err
	}

	if s.dataPlaneMng, err = data_plane.NewManager(dpModules); err != nil {
		return err
	}

	// initializing control plane related modules.
	cpModules := control_plane.ServicerModules{
		DataPlaneClient: s,
	}

	if cpModules.ConfigLoader, err = MakeConfigLoader(configPath); err != nil {
		return err
	}

	cpModules.FwmarkManager = fwmark.NewManager(defaultMaxFwmark, defaultFwmarkBase)

	if cpModules.DnsResolver, err = dns_resolver.NewSystemResolver(maxDnsResolveTime); err != nil {
		return err
	}

	cpModules.DiscoveryFactory = control_plane.NewDiscoveryFactory()

	cpModules.CheckerFactory = control_plane.NewHealthCheckerFactory(
		control_plane.BaseHealthCheckerFactoryParams{
			FwmarkManager: cpModules.FwmarkManager,
		})

	if s.controlPlaneMng, err = control_plane.NewControlPlaneServicer(ctx, cpModules, DefaultSendInterval); err != nil {
		return err
	}

	return nil
}

// emit ipvs service and realserver stats.
func (s *Service) statsLoop(ctx context.Context) {
	ticker := time.NewTicker(statsEmitInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			glog.Info("Closing statsLoop")
			return
		case <-ticker.C:
			if err := s.dataPlaneMng.EmitStats(); err != nil {
				glog.Errorf("Fails to emit stats: %v", err)
			}
		}
	}
}
