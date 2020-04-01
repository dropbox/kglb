package data_plane

import (
	. "gopkg.in/check.v1"
)

type UtilsSuite struct {
}

var _ = Suite(&UtilsSuite{})

func (m *ManagerSuite) TestDiffMetric(c *C) {
	c.Assert(int(diffMetric(0, 0)), Equals, 0)
	c.Assert(int(diffMetric(10, 10)), Equals, 0)
	c.Assert(int(diffMetric(25, 10)), Equals, 15)
	c.Assert(int(diffMetric(10, 25)), Equals, 10)
}
