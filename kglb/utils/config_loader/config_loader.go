package config_loader

type ConfigProvider interface {
	// The default configuration when the config file (or the locally cached
	// config file) is absent.  The default configuration must be non-nil and
	// valid (i.e., Validate(cfg) returns nil).
	Default() interface{}

	// Given a config file content, return the parsed config object (or error).
	// The parsed config object must be non-nil.
	Parse(content []byte) (cfg interface{}, err error)

	// Validate ensures the config is semantically correct.  If the config is
	// invalid, the config loader will simply ignore the config.
	Validate(cfg interface{}) error

	// The config loader will only sent updates when the config file's
	// content has modified.
	Equals(cfg1 interface{}, cfg2 interface{}) bool
}

// Basic interface for config loader.
type ConfigLoader interface {
	// Config updates channel.
	Updates() <-chan interface{}
	// Stopping config loader and closing its update channel.
	Stop()
}
