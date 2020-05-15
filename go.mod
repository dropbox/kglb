module dropbox

go 1.13

require (
	dropbox/proto/kglb v0.0.0-00010101000000-000000000000
	github.com/dropbox/godropbox v0.0.0-20200221053928-caf2e8d91700 // indirect
	github.com/gogo/protobuf v1.3.1
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.5
	github.com/hkwi/nlgo v0.0.0-20190926025335-08733afbfe04 // indirect
	github.com/miekg/dns v1.1.27
	github.com/mqliang/libipvs v0.0.0-20181031074626-20f197c976a3
	github.com/prometheus/client_golang v1.3.0
	github.com/stretchr/testify v1.3.0
	github.com/vishvananda/netlink v1.0.0
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df // indirect
	godropbox v0.0.0-00010101000000-000000000000
	golang.org/x/net v0.0.0-20200202094626-16171245cfb2
	golang.org/x/sys v0.0.0-20191220142924-d4481acd189f
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15
)

replace (
	dropbox/proto/kglb => ./proto/dropbox/proto/kglb
	godropbox => github.com/dropbox/godropbox v0.0.0-20200228041828-52ad444d3502
)
