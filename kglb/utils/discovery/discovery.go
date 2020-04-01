package discovery

type DiscoveryResolver interface {
	// Discovery Resolver id.
	GetId() string

	// Get current state of the resolver.
	GetState() DiscoveryState

	// Get changes of the resolver through channel.
	Updates() <-chan DiscoveryState

	// Close/Stop resolver.
	Close()

	// Check if the item discovers exactly the same things.
	Equal(item DiscoveryResolver) bool
}
