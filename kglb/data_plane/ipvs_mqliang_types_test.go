package data_plane

import (
	"github.com/mqliang/libipvs"
	. "gopkg.in/check.v1"

	kglb_pb "dropbox/proto/kglb"
	. "godropbox/gocheck2"
)

type ConvertTypesTestSuite struct{}

var _ = Suite(&ConvertTypesTestSuite{})

func (ts *ConvertTypesTestSuite) TestKglbFlags(c *C) {
	// flags not set
	flags, err := toLibIpvsFlags(nil)
	c.Assert(err, NoErr)
	c.Assert(flags, DeepEqualsPretty, libipvs.Flags{Mask: ^uint32(0)})

	// single flag
	flags, err = toLibIpvsFlags([]kglb_pb.IpvsService_Flag{kglb_pb.IpvsService_SH_PORT})
	c.Assert(err, NoErr)
	c.Assert(flags, DeepEqualsPretty,
		libipvs.Flags{
			Flags: libipvs.IP_VS_SVC_F_SCHED_SH_PORT,
			Mask:  ^uint32(0),
		})

	// multiple flags
	flags, err = toLibIpvsFlags(
		[]kglb_pb.IpvsService_Flag{
			kglb_pb.IpvsService_SH_PORT,
			kglb_pb.IpvsService_SH_FALLBACK,
		})
	c.Assert(err, NoErr)
	c.Assert(flags, DeepEqualsPretty,
		libipvs.Flags{
			Flags: libipvs.IP_VS_SVC_F_SCHED_SH_PORT | libipvs.IP_VS_SVC_F_SCHED_SH_FALLBACK,
			Mask:  ^uint32(0),
		})

	// unsupported flag (IpvsService_EMPTY)
	_, err = toLibIpvsFlags([]kglb_pb.IpvsService_Flag{kglb_pb.IpvsService_EMPTY})
	c.Assert(err, MultilineErrorMatches, "Unsupported kglb flag: EMPTY")
	_, err = toLibIpvsFlags([]kglb_pb.IpvsService_Flag{kglb_pb.IpvsService_SH_PORT, kglb_pb.IpvsService_EMPTY})
	c.Assert(err, MultilineErrorMatches, "Unsupported kglb flag: EMPTY")
}

func (ts *ConvertTypesTestSuite) TestlibIpvsFlags(c *C) {
	// mask zero value
	_, err := toKglbFlags(libipvs.Flags{})
	c.Assert(err, MultilineErrorMatches, "Unsupported mask: 0")

	// incorrect mask
	_, err = toKglbFlags(libipvs.Flags{Mask: 10})
	c.Assert(err, MultilineErrorMatches, "Unsupported mask: 10")

	// no flags
	flags, err := toKglbFlags(libipvs.Flags{Mask: ^uint32(0)})
	c.Assert(err, NoErr)
	c.Assert(flags, DeepEqualsPretty, []kglb_pb.IpvsService_Flag{})

	// single flag
	flags, err = toKglbFlags(
		libipvs.Flags{
			Flags: libipvs.IP_VS_SVC_F_SCHED_SH_PORT,
			Mask:  ^uint32(0),
		})
	c.Assert(err, NoErr)
	c.Assert(flags, DeepEqualsPretty, []kglb_pb.IpvsService_Flag{kglb_pb.IpvsService_SH_PORT})

	// multiple flags
	flags, err = toKglbFlags(
		libipvs.Flags{
			Flags: libipvs.IP_VS_SVC_F_SCHED_SH_PORT | libipvs.IP_VS_SVC_F_SCHED_SH_FALLBACK,
			Mask:  ^uint32(0),
		})
	c.Assert(err, NoErr)
	c.Assert(flags, DeepEqualsPretty, []kglb_pb.IpvsService_Flag{
		kglb_pb.IpvsService_SH_FALLBACK,
		kglb_pb.IpvsService_SH_PORT,
	})

	// unknown flag
	_, err = toKglbFlags(
		libipvs.Flags{
			Flags: 64,
			Mask:  ^uint32(0),
		})
	c.Assert(err, MultilineErrorMatches, "Unsupported flags left: 64")

	// flag combination
	_, err = toKglbFlags(
		libipvs.Flags{
			Flags: libipvs.IP_VS_SVC_F_SCHED_SH_PORT | 128,
			Mask:  ^uint32(0),
		})
	c.Assert(err, MultilineErrorMatches, "Unsupported flags left: 128")
}
