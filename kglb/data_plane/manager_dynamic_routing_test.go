package data_plane

import (
	"time"

	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
	. "godropbox/gocheck2"
)

type DynamicRoutingManagerSuite struct {
	// Mock Bgp module.
	mockBgp *MockBgpModule
	// DynamicRoutingManager.
	manager *DynamicRoutingManager
}

var _ = Suite(&DynamicRoutingManagerSuite{})

func (m *DynamicRoutingManagerSuite) SetUpTest(c *C) {
	m.mockBgp = NewMockBgpModuleWithState().(*MockBgpModule)

	params := DynamicRoutingManagerParams{
		Bgp: m.mockBgp,
	}

	var err error
	m.manager, err = NewDynamicRoutingManager(params)
	c.Assert(err, IsNil)
}

// Advertise routes.
func (m *DynamicRoutingManagerSuite) TestHoldTimeouts(c *C) {
	err := m.manager.AdvertiseRoutes(
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{
								Ipv4: "10.0.0.2",
							},
						},
						Prefixlen:  32,
						HoldTimeMs: 51,
					},
				},
			},
		})
	c.Assert(err, IsNil)
	c.Assert(len(m.manager.holdTimeouts), Equals, 1)
	key := &kglb_pb.IP{
		Address: &kglb_pb.IP_Ipv4{Ipv4: "10.0.0.2"},
	}
	c.Assert(m.manager.holdTimeouts[key.String()], Equals, 51*time.Millisecond)

	holdTime, err := m.manager.withdrawRouteLocked(
		&kglb_pb.DynamicRoute{
			Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
				BgpAttributes: &kglb_pb.BgpRouteAttributes{
					LocalAsn:  10,
					PeerAsn:   20,
					Community: "my_community",
					Prefix: &kglb_pb.IP{
						Address: &kglb_pb.IP_Ipv4{
							Ipv4: "10.0.0.2",
						},
					},
					Prefixlen:  32,
					HoldTimeMs: 51,
				},
			},
		})
	c.Assert(err, IsNil)
	c.Assert(holdTime, Equals, 51*time.Millisecond)
	c.Assert(len(m.manager.holdTimeouts), Equals, 0)

	err = m.manager.AdvertiseRoutes(
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{
								Ipv4: "10.0.0.2",
							},
						},
						Prefixlen:  32,
						HoldTimeMs: 51,
					},
				},
			},
		})
	c.Assert(err, IsNil)

	// withdrawing route.
	startTime := time.Now()
	err = m.manager.WithdrawRoutes(
		[]*kglb_pb.DynamicRoute{
			{
				Attributes: &kglb_pb.DynamicRoute_BgpAttributes{
					BgpAttributes: &kglb_pb.BgpRouteAttributes{
						LocalAsn:  10,
						PeerAsn:   20,
						Community: "my_community",
						Prefix: &kglb_pb.IP{
							Address: &kglb_pb.IP_Ipv4{
								Ipv4: "10.0.0.2",
							},
						},
						Prefixlen:  32,
						HoldTimeMs: 51,
					},
				},
			},
		})
	elapsed := time.Since(startTime)
	c.Assert(err, IsNil)
	c.Assert(len(m.manager.holdTimeouts), Equals, 0)
	c.Assert(elapsed/time.Millisecond, GreaterThan, 50)

}
