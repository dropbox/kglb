package fwmark

import (
	. "gopkg.in/check.v1"
)

var _ = Suite(&FwmarkManagerSuite{})

func (s *FwmarkManagerSuite) TestGetAndRelease(c *C) {
	manager := NewManager(2, 10)

	// new fwmark
	fwmark, err := manager.AllocateFwmark("10.0.0.1")
	c.Assert(err, IsNil)
	c.Assert(fwmark, Equals, uint32(10))

	// trying to get non allocated fwmark
	_, err = manager.GetAllocatedFwmark("10.0.0.2")
	c.Assert(err, NotNil)

	// another fwmark
	fwmark, err = manager.AllocateFwmark("10.0.0.2")
	c.Assert(err, IsNil)
	c.Assert(fwmark, Equals, uint32(11))

	// bump refcount for existing
	fwmark, err = manager.AllocateFwmark("10.0.0.1")
	c.Assert(err, IsNil)
	c.Assert(fwmark, Equals, uint32(10))

	// get allocated fwmark w/o refcounter bumping
	fwmark, err = manager.GetAllocatedFwmark("10.0.0.1")
	c.Assert(err, IsNil)
	c.Assert(fwmark, Equals, uint32(10))

	c.Assert(len(manager.fwmarksPool), Equals, 0)

	// no more fwmarks in free pool
	_, err = manager.AllocateFwmark("10.0.0.3")
	c.Assert(err, NotNil)

	// releasing one w/ refcount 2. 2 times no error,
	// and error on 3rd time
	c.Assert(len(manager.fwmarksPool), Equals, 0)

	err = manager.ReleaseFwmark("10.0.0.1")
	c.Assert(err, IsNil)

	err = manager.ReleaseFwmark("10.0.0.1")
	c.Assert(err, IsNil)

	c.Assert(len(manager.fwmarksPool), Equals, 1)

	err = manager.ReleaseFwmark("10.0.0.1")
	c.Assert(err, NotNil)
}
