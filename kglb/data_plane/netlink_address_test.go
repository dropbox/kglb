package data_plane

import (
	"net"

	"github.com/vishvananda/netlink"
	. "gopkg.in/check.v1"

	"godropbox/errors"
)

type NetlinkAddrSuite struct {
}

var _ = Suite(&NetlinkAddrSuite{})

func (m *NetlinkAddrSuite) TestList(c *C) {
	nlApi := &netlinkFunc{
		AddrAdd:   nil,
		AddrDel:   nil,
		AddrList:  nil,
		ParseAddr: nil,
		LinkByName: func(name string) (netlink.Link, error) {
			return nil, errors.New("LinkByName() fails")
		},
	}

	module, err := newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	addrs, err := module.List("lo1")
	c.Assert(addrs, IsNil)
	c.Assert(err, NotNil)

	// check address family conversion.
	nlApi = &netlinkFunc{
		AddrAdd: nil,
		AddrDel: nil,
		AddrList: func(link netlink.Link, family int) ([]netlink.Addr, error) {
			//c.Assert(family, Equals, netlink.FAMILY_V6)
			_, testIpNet1, err := net.ParseCIDR("::1/32")
			c.Assert(err, IsNil)
			_, testIpNet2, err := net.ParseCIDR("::2/32")
			c.Assert(err, IsNil)

			if family == netlink.FAMILY_V6 {
				return []netlink.Addr{
					{IPNet: testIpNet1},
					{IPNet: testIpNet2},
				}, nil
			} else {
				return nil, nil
			}
		},
		ParseAddr: nil,
		LinkByName: func(name string) (netlink.Link, error) {
			c.Assert(name, Equals, "lo1")
			return &netlink.Dummy{}, nil
		},
	}
	// module with new set of mock netlink funcs.
	module, err = newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	addrs, err = module.List("lo1")
	c.Assert(err, IsNil)
	c.Assert(len(addrs), Equals, 2)
	for _, addr := range addrs {
		c.Assert(addr.To16(), NotNil)
	}

	// 2. ipv4
	nlApi = &netlinkFunc{
		AddrAdd: nil,
		AddrDel: nil,
		AddrList: func(link netlink.Link, family int) ([]netlink.Addr, error) {
			//c.Assert(family, Equals, netlink.FAMILY_V4)
			testIpNet1, err := netlink.ParseAddr("192.168.10.1/32")
			c.Assert(err, IsNil)
			testIpNet2, err := netlink.ParseAddr("192.168.10.2/32")
			c.Assert(err, IsNil)
			if family == netlink.FAMILY_V4 {
				return []netlink.Addr{
					*testIpNet1,
					*testIpNet2,
				}, nil
			} else {
				return nil, nil
			}

		},
		ParseAddr: nil,
		LinkByName: func(name string) (netlink.Link, error) {
			c.Assert(name, Equals, "lo2")
			return &netlink.Dummy{}, nil
		},
	}
	// module with new set of mock netlink funcs.
	module, err = newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	addrs, err = module.List("lo2")
	c.Assert(err, IsNil)
	c.Assert(len(addrs), Equals, 2)
	for _, addr := range addrs {
		c.Assert(addr.To4(), NotNil)
	}
}

func (m *NetlinkAddrSuite) TestIsExists(c *C) {
	// 1. IPv4
	nlApi := &netlinkFunc{
		AddrAdd: nil,
		AddrDel: nil,
		AddrList: func(link netlink.Link, family int) ([]netlink.Addr, error) {
			//c.Assert(family, Equals, netlink.FAMILY_V4)
			testIpNet1, err := netlink.ParseAddr("192.168.10.1/32")
			c.Assert(err, IsNil)
			testIpNet2, err := netlink.ParseAddr("192.168.10.2/32")
			c.Assert(err, IsNil)
			return []netlink.Addr{
				*testIpNet1,
				*testIpNet2,
			}, nil
		},
		ParseAddr: nil,
		LinkByName: func(name string) (netlink.Link, error) {
			c.Assert(name, Equals, "lo2")
			return &netlink.Dummy{}, nil
		},
	}
	// module with new set of mock netlink funcs.
	module, err := newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	exists, err := module.IsExists(net.ParseIP("192.168.10.1"), "lo2")
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	exists, err = module.IsExists(net.ParseIP("192.168.20.1"), "lo2")
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	// 2. IPv6
	nlApi = &netlinkFunc{
		AddrAdd: nil,
		AddrDel: nil,
		AddrList: func(link netlink.Link, family int) ([]netlink.Addr, error) {
			//c.Assert(family, Equals, netlink.FAMILY_V6)
			testIpNet1, err := netlink.ParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7334/32")
			c.Assert(err, IsNil)
			testIpNet2, err := netlink.ParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7335/32")
			c.Assert(err, IsNil)
			return []netlink.Addr{
				*testIpNet1,
				*testIpNet2,
			}, nil
		},
		ParseAddr: nil,
		LinkByName: func(name string) (netlink.Link, error) {
			c.Assert(name, Equals, "lo2")
			return &netlink.Dummy{}, nil
		},
	}
	// module with new set of mock netlink funcs.
	module, err = newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	exists, err = module.IsExists(net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334"), "lo2")
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	exists, err = module.IsExists(net.ParseIP("1001:0db8:85a3:0000:0000:8a2e:0370:7334"), "lo2")
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)
}

func (m *NetlinkAddrSuite) TestAddAddr(c *C) {
	// 1. ipv4
	nlApi := &netlinkFunc{
		AddrAdd: func(link netlink.Link, addr *netlink.Addr) error {
			c.Assert(addr.IP.String(), Equals, "192.168.20.1")
			return nil
		},
		AddrDel: nil,
		AddrList: func(link netlink.Link, family int) ([]netlink.Addr, error) {
			//c.Assert(family, Equals, netlink.FAMILY_V4)
			testIpNet1, err := netlink.ParseAddr("192.168.10.1/32")
			c.Assert(err, IsNil)
			testIpNet2, err := netlink.ParseAddr("192.168.10.2/32")
			c.Assert(err, IsNil)
			return []netlink.Addr{
				*testIpNet1,
				*testIpNet2,
			}, nil
		},
		ParseAddr: netlink.ParseAddr,
		LinkByName: func(name string) (netlink.Link, error) {
			c.Assert(name, Equals, "lo3")
			return &netlink.Dummy{}, nil
		},
	}
	// module with new set of mock netlink funcs.
	module, err := newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	// call fails because of already existent address
	err = module.Add(net.ParseIP("192.168.10.1"), "lo3")
	c.Assert(err, NotNil)

	err = module.Add(net.ParseIP("192.168.20.1"), "lo3")
	c.Assert(err, IsNil)

	// 2. IPv6
	nlApi = &netlinkFunc{
		AddrAdd: func(link netlink.Link, addr *netlink.Addr) error {
			c.Assert(
				addr.IP.Equal(net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:9334")),
				Equals,
				true)
			return nil
		},
		AddrDel: nil,
		AddrList: func(link netlink.Link, family int) ([]netlink.Addr, error) {
			//c.Assert(family, Equals, netlink.FAMILY_V6)
			testIpNet1, err := netlink.ParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7334/32")
			c.Assert(err, IsNil)
			testIpNet2, err := netlink.ParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7335/32")
			c.Assert(err, IsNil)
			return []netlink.Addr{
				*testIpNet1,
				*testIpNet2,
			}, nil
		},
		ParseAddr: netlink.ParseAddr,
		LinkByName: func(name string) (netlink.Link, error) {
			c.Assert(name, Equals, "lo3")
			return &netlink.Dummy{}, nil
		},
	}
	// module with new set of mock netlink funcs.
	module, err = newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	// call fails because of already existent address
	err = module.Add(net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334"), "lo3")
	c.Assert(err, NotNil)

	err = module.Add(net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:9334"), "lo3")
	c.Assert(err, IsNil)
}

func (m *NetlinkAddrSuite) TestDelAddr(c *C) {
	// 1. ipv4
	nlApi := &netlinkFunc{
		AddrAdd: nil,
		AddrDel: func(link netlink.Link, addr *netlink.Addr) error {
			c.Assert(addr.IP.String(), Equals, "192.168.10.1")
			return nil
		},
		AddrList: func(link netlink.Link, family int) ([]netlink.Addr, error) {
			//c.Assert(family, Equals, netlink.FAMILY_V4)
			testIpNet1, err := netlink.ParseAddr("192.168.10.1/32")
			c.Assert(err, IsNil)
			testIpNet2, err := netlink.ParseAddr("192.168.10.2/32")
			c.Assert(err, IsNil)
			return []netlink.Addr{
				*testIpNet1,
				*testIpNet2,
			}, nil
		},
		ParseAddr: netlink.ParseAddr,
		LinkByName: func(name string) (netlink.Link, error) {
			c.Assert(name, Equals, "lo3")
			return &netlink.Dummy{}, nil
		},
	}
	// module with new set of mock netlink funcs.
	module, err := newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	// call fails because addr doesn't exist
	err = module.Delete(net.ParseIP("192.168.20.1"), "lo3")
	c.Assert(err, NotNil)

	err = module.Delete(net.ParseIP("192.168.10.1"), "lo3")
	c.Assert(err, IsNil)

	// 2. IPv6
	nlApi = &netlinkFunc{
		AddrAdd: nil,
		AddrDel: func(link netlink.Link, addr *netlink.Addr) error {
			c.Assert(
				addr.IP.Equal(net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334")),
				Equals,
				true)
			return nil
		},
		AddrList: func(link netlink.Link, family int) ([]netlink.Addr, error) {
			//c.Assert(family, Equals, netlink.FAMILY_V6)
			testIpNet1, err := netlink.ParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7334/32")
			c.Assert(err, IsNil)
			testIpNet2, err := netlink.ParseAddr("2001:0db8:85a3:0000:0000:8a2e:0370:7335/32")
			c.Assert(err, IsNil)
			return []netlink.Addr{
				*testIpNet1,
				*testIpNet2,
			}, nil
		},
		ParseAddr: netlink.ParseAddr,
		LinkByName: func(name string) (netlink.Link, error) {
			c.Assert(name, Equals, "lo3")
			return &netlink.Dummy{}, nil
		},
	}
	// module with new set of mock netlink funcs.
	module, err = newNetLinkAddress(nlApi)
	c.Assert(err, IsNil)

	// call fails because addr doesn't exist
	err = module.Delete(net.ParseIP("1001:0db8:85a3:0000:0000:8a2e:0370:7334"), "lo3")
	c.Assert(err, NotNil)

	err = module.Delete(net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334"), "lo3")
	c.Assert(err, IsNil)
}
