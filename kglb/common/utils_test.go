package common

import (
	. "gopkg.in/check.v1"

	pb "dropbox/proto/kglb"
)

type UtilsSuite struct{}

var _ = Suite(&UtilsSuite{})

func (s *UtilsSuite) TestAliveUpstreamsRatio(c *C) {
	c.Assert(AliveUpstreamsRatio([]*pb.UpstreamState{
		{Weight: 100}, {Weight: 100},
	}), Equals, float32(1))
	c.Assert(AliveUpstreamsRatio([]*pb.UpstreamState{
		{Weight: 100}, {Weight: 0},
	}), Equals, float32(0.5))
	c.Assert(AliveUpstreamsRatio([]*pb.UpstreamState{
		{Weight: 0}, {Weight: 0},
	}), Equals, float32(0))
}
