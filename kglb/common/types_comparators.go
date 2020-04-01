package common

import (
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"

	"dropbox/kglb/utils/comparable"
	kglb_pb "dropbox/proto/kglb"
)

var (
	DataPlaneStateComparable = &comparable.ComparableImpl{
		// TODO(dkopytkov): KeyFunc is unhelpful for now because key will be
		// always different for any change.
		KeyFunc: func(item interface{}) string {
			return item.(*kglb_pb.DataPlaneState).String()
		},
		EqualFunc: func(item1, item2 interface{}) bool {
			dpState1 := item1.(*kglb_pb.DataPlaneState)
			dpState2 := item2.(*kglb_pb.DataPlaneState)

			// comparing BalancerStates.
			balancersDiff := CompareBalancerState(
				dpState1.GetBalancers(),
				dpState2.GetBalancers())

			// skipping futher check when there is change in balancers.
			if balancersDiff.IsChanged() {
				return false
			}

			// comparing DynamicRoute.
			dynamicRoutesDiff := CompareDynamicRouting(
				dpState1.GetDynamicRoutes(),
				dpState2.GetDynamicRoutes())

			if dynamicRoutesDiff.IsChanged() {
				return false
			}

			// compare local link addresses
			localLinkAddrDiff := CompareLocalLinkAddresses(
				dpState1.GetLinkAddresses(),
				dpState2.GetLinkAddresses())

			if localLinkAddrDiff.IsChanged() {
				return false
			}

			return true
		},
	}

	// Comparable implementation for BgpRoutingAttributes.
	BgpRoutingAttributesComparable = &comparable.ComparableImpl{
		KeyFunc: func(item interface{}) string {
			attr := item.(*kglb_pb.BgpRouteAttributes)
			// construct a key.
			community := strings.Replace(
				attr.GetCommunity(),
				",",
				" ",
				-1)
			return fmt.Sprintf(
				"%d:%d:%s:%v:%v",
				attr.GetLocalAsn(),
				attr.GetPeerAsn(),
				community,
				attr.GetPrefix().String(),
				attr.GetPrefixlen())
		},
		EqualFunc: func(item1, item2 interface{}) bool {
			// compare items.
			attr1 := item1.(*kglb_pb.BgpRouteAttributes)
			// construct a key.
			community1 := strings.Replace(
				attr1.GetCommunity(),
				",",
				" ",
				-1)
			attr2 := item2.(*kglb_pb.BgpRouteAttributes)
			// construct a key.
			community2 := strings.Replace(
				attr2.GetCommunity(),
				",",
				" ",
				-1)
			return attr1.GetLocalAsn() == attr2.GetLocalAsn() &&
				attr1.GetPeerAsn() == attr2.GetPeerAsn() &&
				community1 == community2 &&
				attr1.GetPrefix().String() == attr2.GetPrefix().String() &&
				attr1.GetPrefixlen() == attr2.GetPrefixlen()
		},
	}

	// Comparable implementation for DynamicRouting.
	DynamicRoutingComparable = &comparable.ComparableImpl{
		KeyFunc: func(item interface{}) string {
			return BgpRoutingAttributesComparable.KeyFunc(
				item.(*kglb_pb.DynamicRoute).GetBgpAttributes())
		},
		EqualFunc: func(item1, item2 interface{}) bool {
			return BgpRoutingAttributesComparable.EqualFunc(
				item1.(*kglb_pb.DynamicRoute).GetBgpAttributes(),
				item2.(*kglb_pb.DynamicRoute).GetBgpAttributes())
		},
	}

	// Comparable implementation for BalancerState.
	BalancerStateComparable = &comparable.ComparableImpl{
		// key based on LoadBalancerService.
		KeyFunc: func(item interface{}) string {
			return LoadBalancerServiceComparable.Key(
				item.(*kglb_pb.BalancerState).GetLbService())
		},
		EqualFunc: func(item1, item2 interface{}) bool {
			balancer1 := item1.(*kglb_pb.BalancerState)
			balancer2 := item2.(*kglb_pb.BalancerState)

			// compare just name and reals since key equality means equality of
			// LoadBalancerService.

			//compare reals
			realsDiff := CompareUpstreamState(
				balancer1.GetUpstreams(),
				balancer2.GetUpstreams())

			return balancer1.GetName() == balancer2.GetName() &&
				!realsDiff.IsChanged()
		},
	}

	// Comparable implementation for LoadBalancerService.
	LoadBalancerServiceComparable = &comparable.ComparableImpl{
		// all fields of the LoadBalancerService proto is part of key.
		KeyFunc: func(item interface{}) string {
			srv := item.(*kglb_pb.LoadBalancerService)
			return srv.String()
		},

		EqualFunc: func(item1, item2 interface{}) bool {
			return proto.Equal(
				item1.(*kglb_pb.LoadBalancerService),
				item2.(*kglb_pb.LoadBalancerService))
		},
	}

	// Comparable implementation for UpstreamState.
	UpstreamStateComparable = &comparable.ComparableImpl{
		// define key as address:port string
		KeyFunc: func(item interface{}) string {
			upstreamItem := item.(*kglb_pb.UpstreamState)

			return fmt.Sprintf(
				"%s:%d",
				KglbAddrToNetIp(upstreamItem.GetAddress()),
				upstreamItem.GetPort())
		},

		EqualFunc: func(item1, item2 interface{}) bool {
			return proto.Equal(
				item1.(*kglb_pb.UpstreamState),
				item2.(*kglb_pb.UpstreamState))
		},
	}

	LocalLinkAddrComparable = &comparable.ComparableImpl{
		KeyFunc: func(item interface{}) string {
			linkAddressItem := item.(*kglb_pb.LinkAddress)

			return fmt.Sprintf(
				"%s:%s",
				linkAddressItem.GetLinkName(),
				KglbAddrToNetIp(linkAddressItem.GetAddress()),
			)
		},

		EqualFunc: func(item1, item2 interface{}) bool {
			return proto.Equal(
				item1.(*kglb_pb.LinkAddress),
				item2.(*kglb_pb.LinkAddress))
		},
	}
)

func CompareDynamicRouting(
	oldSet,
	newSet []*kglb_pb.DynamicRoute) *comparable.ComparableResult {

	return comparable.CompareArrays(
		DynamicRoutingConv(oldSet),
		DynamicRoutingConv(newSet),
		DynamicRoutingComparable)
}

func DynamicRoutingConv(set []*kglb_pb.DynamicRoute) []interface{} {
	setConv := make([]interface{}, len(set))
	for i, val := range set {
		setConv[i] = val
	}

	return setConv
}

func DynamicRoutingConvBack(set []interface{}) []*kglb_pb.DynamicRoute {
	setConv := make([]*kglb_pb.DynamicRoute, len(set))
	for i, val := range set {
		setConv[i] = val.(*kglb_pb.DynamicRoute)
	}

	return setConv
}

func CompareBalancerState(
	oldSet,
	newSet []*kglb_pb.BalancerState) *comparable.ComparableResult {

	return comparable.CompareArrays(
		BalancerStateConv(oldSet),
		BalancerStateConv(newSet),
		BalancerStateComparable)
}

func BalancerStateConv(set []*kglb_pb.BalancerState) []interface{} {
	setConv := make([]interface{}, len(set))
	for i, val := range set {
		setConv[i] = val
	}

	return setConv
}

func BalancerStateConvBack(set []interface{}) []*kglb_pb.BalancerState {
	setConv := make([]*kglb_pb.BalancerState, len(set))
	for i, val := range set {
		setConv[i] = val.(*kglb_pb.BalancerState)
	}

	return setConv
}

func CompareLoadBalancerService(
	oldSet,
	newSet []*kglb_pb.LoadBalancerService) *comparable.ComparableResult {

	return comparable.CompareArrays(
		LoadBalancerServiceConv(oldSet),
		LoadBalancerServiceConv(newSet),
		LoadBalancerServiceComparable)
}

func LoadBalancerServiceConv(set []*kglb_pb.LoadBalancerService) []interface{} {
	setConv := make([]interface{}, len(set))
	for i, val := range set {
		setConv[i] = val
	}

	return setConv
}

func CompareUpstreamState(
	oldSet,
	newSet []*kglb_pb.UpstreamState) *comparable.ComparableResult {

	return comparable.CompareArrays(
		UpstreamStateConv(oldSet),
		UpstreamStateConv(newSet),
		UpstreamStateComparable)
}

func UpstreamStateConv(set []*kglb_pb.UpstreamState) []interface{} {
	setConv := make([]interface{}, len(set))
	for i, val := range set {
		setConv[i] = val
	}

	return setConv
}
func UpstreamStateConvBack(set []interface{}) []*kglb_pb.UpstreamState {
	setConv := make([]*kglb_pb.UpstreamState, len(set))
	for i, val := range set {
		setConv[i] = val.(*kglb_pb.UpstreamState)
	}

	return setConv
}

func CompareLocalLinkAddresses(
	oldSet,
	newSet []*kglb_pb.LinkAddress) *comparable.ComparableResult {

	return comparable.CompareArrays(
		LinkAddressStateConv(oldSet),
		LinkAddressStateConv(newSet),
		LocalLinkAddrComparable)
}

func LinkAddressStateConv(set []*kglb_pb.LinkAddress) []interface{} {
	setConv := make([]interface{}, len(set))
	for i, val := range set {
		setConv[i] = val
	}

	return setConv
}

func LinkAddressStateConvBack(set []interface{}) []*kglb_pb.LinkAddress {
	setConv := make([]*kglb_pb.LinkAddress, len(set))
	for i, val := range set {
		setConv[i] = val.(*kglb_pb.LinkAddress)
	}

	return setConv
}
