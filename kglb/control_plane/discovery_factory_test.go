package control_plane

import (
	. "gopkg.in/check.v1"

	"dropbox/kglb/utils/discovery"
	pb "dropbox/proto/kglb"
	. "godropbox/gocheck2"
)

type DiscoveryFactorySuite struct{}

var _ = Suite(&DiscoveryFactorySuite{})

func (s *DiscoveryFactorySuite) TestStatic(c *C) {
	factory := NewDiscoveryFactory()

	resolver, err := factory.Resolver(
		c.TestName(),
		"testSetup",
		&pb.UpstreamDiscovery{
			Port: 80,
			Attributes: &pb.UpstreamDiscovery_StaticAttributes{
				StaticAttributes: &pb.StaticDiscoveryAttributes{
					Hosts: []string{"test-host-1"},
				},
			},
		})
	c.Assert(err, IsNil)
	_, ok := resolver.(*discovery.StaticResolver)
	c.Assert(ok, IsTrue)

	// Updating.
	err = factory.Update(resolver, &pb.UpstreamDiscovery{
		Port: 80,
		Attributes: &pb.UpstreamDiscovery_StaticAttributes{
			StaticAttributes: &pb.StaticDiscoveryAttributes{
				Hosts: []string{"test-host-2"},
			},
		},
	})
	c.Assert(err, IsNil)
	c.Assert(resolver.GetState().Equal(
		discovery.DiscoveryState([]*discovery.HostPort{
			discovery.NewHostPort("test-host-2", 80, true),
		})), IsTrue)
}
