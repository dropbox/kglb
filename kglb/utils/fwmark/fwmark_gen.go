package fwmark

import (
	"fmt"

	"godropbox/murmur3"
)

const (
	fwMarkSeed = 18410482
)

type FwmarkParams struct {
	Hostname string
	IP       string
	Port     uint32
}

func (p FwmarkParams) String() string {
	return fmt.Sprintf("%s:%s:%d", p.Hostname, p.IP, p.Port)
}

func GetFwmark(params FwmarkParams) uint32 {
	return murmur3.Hash32([]byte(params.String()), fwMarkSeed)
}
