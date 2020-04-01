package discovery

import (
	. "godropbox/gocheck2"
	. "gopkg.in/check.v1"
)

type HostPortSuite struct{}

var _ = Suite(&HostPortSuite{})

func (s *HostPortSuite) TestHostPortEqual(c *C) {
	c.Assert(
		NewHostPort("host1", 80).Equal(NewHostPort("host1", 80)), IsTrue)
	c.Assert(
		NewHostPort("host1", 80).Equal(NewHostPort("host2", 80)), IsFalse)
	c.Assert(
		NewHostPort("host1", 80).Equal(NewHostPort("host1", 82)), IsFalse)
}

func (s *HostPortSuite) TestDiscoveryStateEqual(c *C) {
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	})), IsTrue)

	// check reorder.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host2", 80),
		NewHostPort("host1", 80),
	})), IsTrue)

	// different port with the same name.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 81),
	})), IsFalse)

	// differente size.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
		NewHostPort("host3", 80),
	})), IsFalse)
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
		NewHostPort("host3", 80),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	})), IsFalse)
}

func (s *HostPortSuite) TestContains(c *C) {
	// true conditions.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	}).Contains(NewHostPort("host2", 80)), IsTrue)

	// false conditions.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	}).Contains(NewHostPort("host2", 81)), IsFalse)
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
	}).Contains(NewHostPort("host3", 80)), IsFalse)

}
