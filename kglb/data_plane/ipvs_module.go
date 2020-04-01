package data_plane

import kglb_pb "dropbox/proto/kglb"

type IpvsModule interface {
	// Add ipvs service with specific VIP:port, proto ("tcp"|"udp") and
	// scheduler.
	AddService(service *kglb_pb.IpvsService) error
	// Delete ipvs service.
	DeleteService(service *kglb_pb.IpvsService) error
	// Get list of existent ipvs Services.
	ListServices() ([]*kglb_pb.IpvsService, []*kglb_pb.Stats, error)
	// Get list of destinations of specific ipvs service
	GetRealServers(service *kglb_pb.IpvsService) ([]*kglb_pb.UpstreamState, []*kglb_pb.Stats, error)

	// Add destinations to the specific ipvs service.
	AddRealServers(service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error
	// Delete destination from specific ipvs service.
	DeleteRealServers(service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error
	// Update destination for specific ipvs service.
	UpdateRealServers(service *kglb_pb.IpvsService, dsts []*kglb_pb.UpstreamState) error
}
