# KgLb ![Build](https://github.com/piscesdk/kglb/workflows/Build/badge.svg) ![Test](https://github.com/piscesdk/kglb/workflows/Test/badge.svg) ![Lint](https://github.com/piscesdk/kglb/workflows/Lint/badge.svg)

KgLb is L4 load balancer built on top of IP_VS.

![KgLb image](doc/kglb.png)

## Requirements
- Go 1.13
- Linux Kernel 4.4+
- protoc 3.6.1+ and protoc-gen-go

## Supported features
- Discovery: static only
- Health Checkers: http, dns, syslog, tcp
- Tunneled health checking through fwmarks.
- Stats exported in prometheus format and available on http://127.0.0.1:5678/stats by default.
- Graceful shutdown.

## Installation
```bash
# Compile protobufs
pushd ./proto
protoc --go_out=. ./dropbox/proto/kglb/healthchecker/healthchecker.proto
protoc --go_out=. ./dropbox/proto/kglb/*.proto

# Compiling
go build -o kglbd.bin ./kglbd
```

## Quick start
Consider very simple example of Read / Load / Attach
```bash
sudo ./kglbd.bin -config=./examples/example_config.json -logtostderr
```
