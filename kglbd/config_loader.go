package main

import (
	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/jsonpb"

	"dropbox/kglb/common"
	"dropbox/kglb/utils/config_loader"
	pb "dropbox/proto/kglb"
)

type ConfigProvider struct{}

func (c *ConfigProvider) Default() interface{} {
	return &pb.ControlPlaneConfig{}
}

func (c *ConfigProvider) Parse(content []byte) (interface{}, error) {
	newConfig := &pb.ControlPlaneConfig{}
	if err := jsonpb.UnmarshalString(string(content), newConfig); err != nil {
		return nil, err
	}
	return newConfig, nil
}

func (c *ConfigProvider) Validate(cfg interface{}) error {
	config := cfg.(*pb.ControlPlaneConfig)
	return common.ValidateControlPlaneConfig(config)
}

func (c *ConfigProvider) Equals(cfg1 interface{}, cfg2 interface{}) bool {
	return proto.Equal(cfg1.(*pb.ControlPlaneConfig), cfg2.(*pb.ControlPlaneConfig))
}

func MakeConfigLoader(configPath string) (config_loader.ConfigLoader, error) {
	return config_loader.NewOneTimeFileLoader(&ConfigProvider{}, configPath)
}
