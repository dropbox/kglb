package health_manager

import (
	. "gopkg.in/check.v1"

	"dropbox/kglb/utils/discovery"
	. "godropbox/gocheck2"
)

type HealthManagerStateSuite struct {
}

var _ = Suite(&HealthManagerStateSuite{})

func (m *HealthManagerStateSuite) TestContains(c *C) {
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(false, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).Contains(discovery.NewHostPort("host1", 80, true)), IsTrue)

	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(false, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).Contains(discovery.NewHostPort("host2", 80, true)), IsTrue)

	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(false, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).Contains(discovery.NewHostPort("host1", 82, true)), IsFalse)
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(false, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).Contains(discovery.NewHostPort("host3", 80, true)), IsFalse)
}

func (m *HealthManagerStateSuite) TestGetEntry(c *C) {
	testEntry := NewHealthManagerEntry(false, "host1", 80)
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			testEntry,
			NewHealthManagerEntry(false, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).GetEntry(discovery.NewHostPort("host1", 80, true)).Equal(&testEntry),
		IsTrue)

	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			testEntry,
			NewHealthManagerEntry(false, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).GetEntry(discovery.NewHostPort("host3", 80, true)), IsNil)
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			testEntry,
			NewHealthManagerEntry(false, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).GetEntry(discovery.NewHostPort("host2", 81, true)), IsNil)
}

func (m *HealthManagerStateSuite) TestString(c *C) {
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(true, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).String(), Equals, "[host1:80/false, host1:81/true, host2:80/false]")
}

func (m *HealthManagerStateSuite) TestIsHealthy(c *C) {
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(true, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).IsHealthy(), IsTrue)

	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(false, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).IsHealthy(), IsFalse)
}

func (m *HealthManagerStateSuite) TestEqual(c *C) {
	// just different order.
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(true, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).Equal(HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(true, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
			NewHealthManagerEntry(false, "host1", 80),
		})), IsTrue)

	// different items.
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(true, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).Equal(HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(true, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
			NewHealthManagerEntry(false, "host3", 80),
		})), IsFalse)
	c.Assert(
		HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(false, "host1", 80),
			NewHealthManagerEntry(true, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
		}).Equal(HealthManagerState([]HealthManagerEntry{
			NewHealthManagerEntry(true, "host1", 81),
			NewHealthManagerEntry(false, "host2", 80),
			NewHealthManagerEntry(false, "host1", 81),
		})), IsFalse)
}
