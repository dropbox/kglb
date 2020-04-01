package main

import (
	"context"
	"flag"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	flagStatusPort := flag.String(
		"status_port",
		"127.0.0.1:5678",
		"status port.")

	flagConfigPath := flag.String(
		"config_path",
		"",
		"full path to the configuration.")
	flag.Parse()

	if len(*flagConfigPath) == 0 {
		glog.Fatal("-config_path is required path.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := NewService(ctx, *flagConfigPath); err != nil {
		glog.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/stats", promhttp.Handler())

	srv := &http.Server{
		Addr:           *flagStatusPort,
		Handler:        mux,
		ReadTimeout:    time.Minute,
		WriteTimeout:   time.Minute,
		MaxHeaderBytes: 1 << 20,
	}

	glog.Fatal(srv.ListenAndServe())
}
