package data_plane

import (
	"github.com/mqliang/libipvs"

	"dropbox/dlog"
	"dropbox/exclog"
	"dropbox/kglb/common"
	kglb_pb "dropbox/proto/kglb"
	"godropbox/errors"
)

// wrapper for go/src/github.com/mqliang/libipvs lib

// wrapper for go/src/github.com/mqliang/libipvs lib
type IpvsMqliang struct {
	// ipvs lib.
	libIpvs libipvs.IPVSHandle
	// DNS related module.
	resolver ResolverModule
}

func NewIpvsMqLiang(resolver ResolverModule) (IpvsModule, error) {

	ipvsLib, err := libipvs.NewIPVSHandle(libipvs.IPVSHandleParams{})
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "fails to init IPVS library"),
			exclog.Critical, "")
		return nil, err
	}

	ipvsModule := &IpvsMqliang{
		libIpvs:  ipvsLib,
		resolver: resolver,
	}

	return ipvsModule, nil
}

// Add service.
func (m *IpvsMqliang) AddService(service *kglb_pb.IpvsService) error {
	libipvsService, err := toLibipvsService(service)
	if err != nil {
		return err
	}

	dlog.Infof("Adding service: %+v", libipvsService)

	err = m.libIpvs.NewService(libipvsService)
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "failed to create IPVS service"),
			exclog.Critical, "")
	}
	return err
}

// Delete ipvs service.
func (m *IpvsMqliang) DeleteService(service *kglb_pb.IpvsService) error {
	libipvsService, err := toLibipvsService(service)
	if err != nil {
		return err
	}
	dlog.Infof("Deleting service: %+v", libipvsService)
	err = m.libIpvs.DelService(libipvsService)
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "failed to delete IPVS service"),
			exclog.Critical, "")
	}
	return err
}

// Get list of existent ipvs Services.
func (m *IpvsMqliang) ListServices() ([]*kglb_pb.IpvsService, []*kglb_pb.Stats, error) {
	services, err := m.libIpvs.ListServices()
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "failed to list IPVS Services"),
			exclog.Critical, "")
		return nil, nil, err
	}

	var result []*kglb_pb.IpvsService
	var servicesStats []*kglb_pb.Stats
	for _, service := range services {
		kglbService, err := tokglbVirtualService(service)
		if err != nil {
			return nil, nil, err
		}

		result = append(result, kglbService)
		servicesStats = append(servicesStats, libipvsTokglbStats(&service.Stats))
	}
	return result, servicesStats, nil
}

// Add real servers to the specific ipvs service.
func (m *IpvsMqliang) AddRealServers(service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error {
	for _, dst := range dsts {
		if err := m.addRealServer(service, dst); err != nil {
			return err
		}
	}
	return nil
}

func (m *IpvsMqliang) addRealServer(service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error {
	libIpvsService, err := toLibipvsService(service)
	if err != nil {
		return err
	}
	libIpvsDst, err := toLibipvsDestination(dst)
	if err != nil {
		return err
	}

	err = m.libIpvs.NewDestination(libIpvsService, libIpvsDst)
	if err != nil {
		exclog.Report(
			errors.Wrapf(
				err,
				"failed to add IPVS real server: %+v, service: %+v",
				dst,
				service),
			exclog.Critical, "")
	}
	return err

}

// Delete real servers from specific ipvs service.
func (m *IpvsMqliang) DeleteRealServers(service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error {
	for _, dst := range dsts {
		if err := m.deleteRealServer(service, dst); err != nil {
			return err
		}
	}
	return nil
}

func (m *IpvsMqliang) deleteRealServer(
	service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error {
	libIpvsService, err := toLibipvsService(service)
	if err != nil {
		return err
	}
	libIpvsDst, err := toLibipvsDestination(dst)
	if err != nil {
		return err
	}

	err = m.libIpvs.DelDestination(libIpvsService, libIpvsDst)
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "failed to delete IPVS real server"),
			exclog.Critical, "")
	}
	return err
}

// Update real servers for specific ipvs service.
func (m *IpvsMqliang) UpdateRealServers(service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error {
	for _, dst := range dsts {
		if err := m.updateRealServer(service, dst); err != nil {
			return err
		}
	}
	return nil
}

func (m *IpvsMqliang) updateRealServer(
	service *kglb_pb.IpvsService, dst *kglb_pb.UpstreamState) error {
	libIpvsService, err := toLibipvsService(service)
	if err != nil {
		return err
	}
	libIpvsDst, err := toLibipvsDestination(dst)
	if err != nil {
		return err
	}

	err = m.libIpvs.UpdateDestination(libIpvsService, libIpvsDst)
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "failed to update IPVS real server"),
			exclog.Critical, "")
	}
	return err
}

// Get list of real servers of specific ipvs service
func (m *IpvsMqliang) GetRealServers(
	service *kglb_pb.IpvsService) ([]*kglb_pb.UpstreamState, []*kglb_pb.Stats, error) {

	libIpvsService, err := toLibipvsService(service)
	if err != nil {
		return nil, nil, err
	}

	libIpvsDsts, err := m.libIpvs.ListDestinations(libIpvsService)
	if err != nil {
		exclog.Report(
			errors.Wrap(err, "failed to list IPVS real servers"),
			exclog.Critical, "")
		return nil, nil, err
	}

	var realServers []*kglb_pb.UpstreamState
	var serversStats []*kglb_pb.Stats
	for _, libIpvsDst := range libIpvsDsts {
		kglbRealServer, err := tokglbRealServer(libIpvsDst)
		if err != nil {
			return nil, nil, err
		}

		rip := common.KglbAddrToNetIp(kglbRealServer.GetAddress())
		hostName, err := m.resolver.ReverseLookup(rip)
		if err != nil {
			exclog.Report(
				errors.Wrapf(
					err,
					"reverse lookup failed for %s address: ",
					rip),
				exclog.Critical, "")
		} else {
			kglbRealServer.Hostname = hostName
		}

		realServers = append(realServers, kglbRealServer)
		dstStats := libipvsTokglbStats(&libIpvsDst.Stats)
		// adding destination specific stats.
		dstStats.ActiveConnsCount = uint64(libIpvsDst.ActiveConns)
		dstStats.InactConnsCount = uint64(libIpvsDst.InactConns)
		dstStats.PersistConnsCount = uint64(libIpvsDst.PersistConns)
		serversStats = append(serversStats, dstStats)
	}

	return realServers, serversStats, nil
}
