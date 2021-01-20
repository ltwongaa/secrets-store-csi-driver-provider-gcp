// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Binary secrets-store-csi-driver-provider-gcp is a plugin for the
// secrets-store-csi-driver for fetching secrets from Google Cloud's Secret
// Manager API.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/GoogleCloudPlatform/secrets-store-csi-driver-provider-gcp/server"
	"google.golang.org/grpc"
	jlogs "k8s.io/component-base/logs/json"
	"k8s.io/klog/v2"
	"sigs.k8s.io/secrets-store-csi-driver/provider/v1alpha1"
)

var (
	kubeconfig    = flag.String("kubeconfig", "", "absolute path to kubeconfig file")
	logFormatJSON = flag.Bool("log-format-json", true, "set log formatter to json")

	version = "dev"
)

func main() {
	klog.InitFlags(nil)
	defer klog.Flush()

	if *logFormatJSON {
		klog.SetLogger(jlogs.JSONLogger)
	}

	flag.Parse()
	ctx := withShutdownSignal(context.Background())

	ua := fmt.Sprintf("secrets-store-csi-driver-provider-gcp/%s", version)
	klog.Infof("starting %s", ua)

	s := &server.Server{
		UA:         ua,
		Kubeconfig: *kubeconfig,
	}

	socketPath := filepath.Join(os.Getenv("TARGET_DIR"), "gcp.sock")
	// Attempt to remove the UDS to handle cases where a previous execution was
	// killed before fully closing the socket listener and unlinking.
	_ = os.Remove(socketPath)

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		klog.ErrorS(err, "unable to listen to unix socket", "path", socketPath)
		klog.Fatalln("unable to start")
	}
	defer l.Close()

	g := grpc.NewServer()
	v1alpha1.RegisterCSIDriverProviderServer(g, s)
	go g.Serve(l)

	<-ctx.Done()
	klog.InfoS("terminating")
	g.GracefulStop()
}

// withShutdownSignal returns a copy of the parent context that will close if
// the process receives termination signals.
func withShutdownSignal(ctx context.Context) context.Context {
	nctx, cancel := context.WithCancel(ctx)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		klog.InfoS("received shutdown signal", "signal", sig)
		cancel()
	}()
	return nctx
}
