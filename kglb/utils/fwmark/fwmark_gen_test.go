package fwmark

import (
	. "gopkg.in/check.v1"

	. "godropbox/gocheck2"
)

type FwmarkManagerSuite struct{}

var _ = Suite(&FwmarkManagerSuite{})

func (s *FwmarkManagerSuite) TestGet(c *C) {
	p1 := FwmarkParams{
		Hostname: "test-hostname-1",
		IP:       "127.0.0.1",
		Port:     80,
	}

	r1 := GetFwmark(p1)
	r2 := GetFwmark(p1)
	c.Assert(r1, Equals, r2)

	p2 := FwmarkParams{
		Hostname: "test-hostname-2",
		IP:       "127.0.0.1",
		Port:     80,
	}

	r3 := GetFwmark(p2)
	c.Assert(r3 == r1, IsFalse)
}
