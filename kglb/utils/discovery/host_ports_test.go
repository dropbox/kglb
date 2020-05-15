package discovery

import (
	. "gopkg.in/check.v1"

	. "godropbox/gocheck2"
)

type HostPortSuite struct{}

var _ = Suite(&HostPortSuite{})

func (s *HostPortSuite) TestHostPortEqual(c *C) {
	c.Assert(
		NewHostPort("host1", 80, true).Equal(NewHostPort("host1", 80, true)), IsTrue)
	c.Assert(
		NewHostPort("host1", 80, true).Equal(NewHostPort("host2", 80, true)), IsFalse)
	c.Assert(
		NewHostPort("host1", 80, true).Equal(NewHostPort("host1", 82, true)), IsFalse)
}

func (s *HostPortSuite) TestDiscoveryStateEqual(c *C) {
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	})), IsTrue)

	// check reorder.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host2", 80, true),
		NewHostPort("host1", 80, true),
	})), IsTrue)

	// different port with the same name.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 81, true),
	})), IsFalse)

	// differente size.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
		NewHostPort("host3", 80, true),
	})), IsFalse)
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
		NewHostPort("host3", 80, true),
	}).Equal(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	})), IsFalse)
}

func (s *HostPortSuite) TestContains(c *C) {
	// true conditions.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	}).Contains(NewHostPort("host2", 80, true)), IsTrue)

	// false conditions.
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	}).Contains(NewHostPort("host2", 81, true)), IsFalse)
	c.Assert(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80, true),
		NewHostPort("host2", 80, true),
	}).Contains(NewHostPort("host3", 80, true)), IsFalse)

}
