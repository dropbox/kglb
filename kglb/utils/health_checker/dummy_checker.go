package health_checker

import (
	hc_pb "dropbox/proto/kglb/healthchecker"
)

// Dummy health checker implementation which always returns true from Check().
type DummyChecker struct {
	params *hc_pb.DummyCheckerAttributes
}

func NewDummyChecker(params *hc_pb.DummyCheckerAttributes) (*DummyChecker, error) {
	return &DummyChecker{params: params}, nil
}

// Performs test and returns true when test was succeed.
func (d *DummyChecker) Check(host string, port int) error {
	return nil
}

func (d *DummyChecker) GetConfiguration() *hc_pb.HealthCheckerAttributes {
	return &hc_pb.HealthCheckerAttributes{
		Attributes: &hc_pb.HealthCheckerAttributes_Dummy{
			Dummy: d.params,
		},
	}
}

var _ HealthChecker = &DummyChecker{}
