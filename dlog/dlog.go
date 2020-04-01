package dlog

import (
	"github.com/golang/glog"
)

type Level = glog.Level

var (
	V               = glog.V
	Stats           = glog.Stats
	Flush           = glog.Flush

	Info          = glog.Info
	Infoln        = glog.Infoln
	Infof         = glog.Infof
	Warning       = glog.Warning
	Warningln     = glog.Warningln
	Warningf      = glog.Warningf
	Error         = glog.Error
	Errorln       = glog.Errorln
	Errorf        = glog.Errorf
	Fatal         = glog.Fatal
	Fatalln    = glog.Fatalln
	Fatalf     = glog.Fatalf
)
