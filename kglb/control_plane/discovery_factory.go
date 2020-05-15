package control_plane

import (
	"fmt"

	"dropbox/kglb/utils/discovery"
	pb "dropbox/proto/kglb"
	"godropbox/errors"
)

var (
	ErrResolverIncompatibleType = fmt.Errorf("incompatible resolver type")
)

// Interface to create and update DiscoveryResolver instance based on
// DiscoveryResolver proto.
type DiscoveryFactory interface {
	// Returns configured instance of DiscoveryResolver based on
	// UpstreamDiscovery proto.
	// TODO(oleg): propagate context to resolver
	Resolver(
		name string,
		setupName string,
		discoveryConf *pb.UpstreamDiscovery) (discovery.DiscoveryResolver, error)

	// Updates Resolver config or returns error when it fails for any reason.
	// errResolverIncompatibleType will be returned when type of resolver
	// instance doesn't match new config.
	Update(
		resolver discovery.DiscoveryResolver,
		conf *pb.UpstreamDiscovery) error
}

type BaseDiscoveryFactory struct{}

func NewDiscoveryFactory() *BaseDiscoveryFactory {

	return &BaseDiscoveryFactory{}
}

// Returns Discovery Resolver instance based on configuration.
func (f *BaseDiscoveryFactory) Resolver(
	name string,
	setupName string,
	conf *pb.UpstreamDiscovery) (discovery.DiscoveryResolver, error) {

	switch attr := conf.Attributes.(type) {
	case *pb.UpstreamDiscovery_StaticAttributes:
		port := int(conf.Port)
		hostPorts := make(
			[]*discovery.HostPort,
			len(attr.StaticAttributes.Hosts))
		for i, host := range attr.StaticAttributes.Hosts {
			hostPorts[i] = discovery.NewHostPort(host, port, true)
		}
		params := discovery.StaticResolverParams{
			Id:          fmt.Sprintf("%s/static", name), // Resolver Id.
			Hosts:       hostPorts,                      // Initial state.
			SetupName:   setupName,
			ServiceName: name,
		}

		return discovery.NewStaticResolver(params)
	default:
		return nil, errors.Newf(
			"DiscoverResolver is not implemented for %s",
			attr)
	}
}

// Updates Resolver config or returns error when it fails.
func (f *BaseDiscoveryFactory) Update(
	resolver discovery.DiscoveryResolver,
	conf *pb.UpstreamDiscovery) error {

	switch attr := conf.Attributes.(type) {
	case *pb.UpstreamDiscovery_StaticAttributes:
		staticResolver, ok := resolver.(*discovery.StaticResolver)
		if !ok {
			return ErrResolverIncompatibleType
		}

		port := int(conf.Port)
		hostPorts := make(
			[]*discovery.HostPort,
			len(attr.StaticAttributes.Hosts))
		for i, host := range attr.StaticAttributes.Hosts {
			hostPorts[i] = discovery.NewHostPort(host, port, true)
		}

		staticResolver.Update(hostPorts)
		return nil
	default:
		return errors.Newf(
			"DiscoverResolver is not implemented for %s",
			attr)
	}
}

var _ DiscoveryFactory = &BaseDiscoveryFactory{}
