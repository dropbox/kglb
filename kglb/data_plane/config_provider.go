package data_plane

import (
	"github.com/gogo/protobuf/proto"
	"godropbox/errors"

	"dropbox/kglb/common"
	"dropbox/kglb/utils/config_loader"
	pb "dropbox/proto/kglb"
)

type ConfigProvider struct {
}

// The default configuration when the config file (or the locally cached
// config file) is absent.  The default configuration must be non-nil and
// valid (i.e., Validate(cfg) returns nil).
func (k *ConfigProvider) Default() interface{} {
	return &pb.DataPlaneState{}
}

// Given a config file content, return the parsed config object (or error).
func (k *ConfigProvider) Parse(content []byte) (interface{}, error) {
	cfg := &pb.DataPlaneState{}
	if err := proto.UnmarshalText(string(content), cfg); err != nil {
		return nil, errors.Wrapf(err, "Failed to parse config: ")
	}
	return cfg, nil
}

// Validate ensures the config is semantically correct.
func (k *ConfigProvider) Validate(abstractCfg interface{}) error {
	state, ok := abstractCfg.(*pb.DataPlaneState)
	if !ok {
		return errors.New("unexpected type of config")
	}

	// Validate config.
	return common.ValidateDataPlaneState(state)
}

// The config loader will only sent updates when the config file's
// content has modified.
func (k *ConfigProvider) Equals(cfg1 interface{}, cfg2 interface{}) bool {
	return common.DataPlaneStateComparable.Equal(cfg1, cfg2)
}

var _ config_loader.ConfigProvider = &ConfigProvider{}
