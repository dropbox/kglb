package health_checker

import (
	. "gopkg.in/check.v1"

	hc_pb "dropbox/proto/kglb/healthchecker"
	. "godropbox/gocheck2"
)

type DummyCheckerSuite struct {
}

var _ = Suite(&DummyCheckerSuite{})

func (m *DummyCheckerSuite) TestDummyChecker(c *C) {
	checker, err := NewDummyChecker(&hc_pb.DummyCheckerAttributes{})
	c.Assert(err, IsNil)
	c.Assert(checker.Check("localhost", 0), IsNil)

	c.Assert(
		checker.GetConfiguration(),
		DeepEqualsPretty,
		&hc_pb.HealthCheckerAttributes{
			Attributes: &hc_pb.HealthCheckerAttributes_Dummy{
				Dummy: &hc_pb.DummyCheckerAttributes{},
			}})
}
