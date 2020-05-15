package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
		"config",
		"",
		"full path to the configuration.")
	flag.Parse()

	if len(*flagConfigPath) == 0 {
		glog.Fatal("-config is required path.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mng, err := NewService(ctx, *flagConfigPath)
	if err != nil {
		glog.Fatal(err)
	}

	// handle signals defined in the array below to perform graceful
	// data plane shutdown.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, []os.Signal{
		syscall.SIGINT,
		syscall.SIGQUIT,
		syscall.SIGTERM,
		syscall.SIGUSR1,
	}...)

	mux := http.NewServeMux()
	mux.Handle("/stats", promhttp.Handler())

	srv := &http.Server{
		Addr:           *flagStatusPort,
		Handler:        mux,
		ReadTimeout:    time.Minute,
		WriteTimeout:   time.Minute,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		glog.Fatal(srv.ListenAndServe())
	}()

	select {
	case sig := <-sigChan:
		glog.Infof("Received '%v', starting shutdown process...", sig)
		ctx.Done()
	}

	mng.Shutdown()
	srv.Close()
}
