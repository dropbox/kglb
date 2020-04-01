package discovery

import (
	"time"

	. "gopkg.in/check.v1"

	. "godropbox/gocheck2"
)

type StaticResolverSuite struct{}

var _ = Suite(&StaticResolverSuite{})

func (s *StaticResolverSuite) TestEqual(c *C) {
	resolver1, err := NewStaticResolver(
		StaticResolverParams{
			Id: "resolver1",
			Hosts: DiscoveryState([]*HostPort{
				NewHostPort("host1", 80),
				NewHostPort("host2", 80),
			}),
		})
	c.Assert(err, IsNil)

	// checking id.
	testResolver, err := NewStaticResolver(
		StaticResolverParams{
			Id: "resolver2",
			Hosts: DiscoveryState([]*HostPort{
				NewHostPort("host1", 80),
				NewHostPort("host2", 80),
			}),
		})
	c.Assert(err, IsNil)
	c.Assert(resolver1.Equal(testResolver), IsFalse)
	testResolver, err = NewStaticResolver(
		StaticResolverParams{
			Id: "resolver1",
			Hosts: DiscoveryState([]*HostPort{
				NewHostPort("host1", 80),
				NewHostPort("host2", 80),
			}),
		})
	c.Assert(err, IsNil)
	c.Assert(resolver1.Equal(testResolver), IsTrue)
	// checking states.
	testResolver, err = NewStaticResolver(
		StaticResolverParams{
			Id: "resolver1",
			Hosts: DiscoveryState([]*HostPort{
				NewHostPort("host1", 80),
			}),
		})
	c.Assert(err, IsNil)
	c.Assert(resolver1.Equal(testResolver), IsFalse)
	testResolver, err = NewStaticResolver(
		StaticResolverParams{
			Id: "resolver1",
			Hosts: DiscoveryState([]*HostPort{
				NewHostPort("host2", 80),
				NewHostPort("host1", 80),
			}),
		})
	c.Assert(err, IsNil)
	c.Assert(resolver1.Equal(testResolver), IsFalse)
	testResolver, err = NewStaticResolver(
		StaticResolverParams{
			Id: "resolver1",
			Hosts: DiscoveryState([]*HostPort{
				NewHostPort("host1", 80),
				NewHostPort("host2", 80),
			}),
		})
	c.Assert(err, IsNil)
	c.Assert(resolver1.Equal(testResolver), IsTrue)
}

func (s *StaticResolverSuite) TestUpdate(c *C) {
	resolver1, err := NewStaticResolver(
		StaticResolverParams{
			Id: "resolver1",
			Hosts: DiscoveryState([]*HostPort{
				NewHostPort("host1", 80),
				NewHostPort("host2", 80),
			}),
		})
	c.Assert(err, IsNil)

	// checking initial state.
	select {
	case state, ok := <-resolver1.Updates():
		c.Assert(ok, IsTrue)
		c.Assert(state, DeepEquals, DiscoveryState([]*HostPort{
			NewHostPort("host1", 80),
			NewHostPort("host2", 80),
		}))
	case <-time.After(time.Second):
		c.Log("timeout to wait update.")
		c.Fail()
	}

	updateChan := resolver1.Updates()
	resolver1.Update(DiscoveryState([]*HostPort{
		NewHostPort("host1", 80),
		NewHostPort("host2", 80),
		NewHostPort("host3", 80),
	}))
	// waiting update though the channel.
	select {
	case newState, ok := <-updateChan:
		c.Assert(ok, IsTrue)
		c.Assert(newState, DeepEquals, DiscoveryState([]*HostPort{
			NewHostPort("host1", 80),
			NewHostPort("host2", 80),
			NewHostPort("host3", 80),
		}))
	case <-time.After(time.Second):
		c.Log("timeout to wait update.")
		c.Fail()
	}

	// Closing.
	resolver1.Close()
	// waiting update though the channel.
	select {
	case _, ok := <-updateChan:
		c.Assert(ok, IsFalse)
	default:
		c.Log("closing fails.")
		c.Fail()
	}
}
