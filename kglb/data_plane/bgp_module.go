package data_plane

import kglb_pb "dropbox/proto/kglb"

// Bgp module.
type BgpModule interface {
	// init speaker with given ASN
	Init(asn uint32) error
	// advertise path.
	Advertise(config *kglb_pb.BgpRouteAttributes) error
	// withdraw path.
	Withdraw(config *kglb_pb.BgpRouteAttributes) error
	// get list of current bgp paths
	ListPaths() ([]*kglb_pb.BgpRouteAttributes, error)
	//	get BGP session state
	IsSessionEstablished() (bool, error)
}
