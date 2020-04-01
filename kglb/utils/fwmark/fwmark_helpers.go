package fwmark

import (
	kglb_pb "dropbox/proto/kglb"
)

func IsFwmarkService(service *kglb_pb.IpvsService) bool {
	switch service.Attributes.(type) {
	case *kglb_pb.IpvsService_FwmarkAttributes:
		return true
	default:
		return false
	}
}
