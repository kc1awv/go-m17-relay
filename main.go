/*
Copyright (C) 2024 Steve Miller KC1AWV

This program is free software: you can redistribute it and/or modify it
under the terms of the GNU General Public License as published by the Free
Software Foundation, either version 3 of the License, or (at your option)
any later version.

This program is distributed in the hope that it will be useful, but WITHOUT
ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
FITNESS FOR A PARTICULAR PURPOSE. See the GNU General Public License for
more details.

You should have received a copy of the GNU General Public License along with
this program. If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"go-m17-relay/config"
	"go-m17-relay/logging"
	"go-m17-relay/metrics"
	"go-m17-relay/relay"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sevlyar/go-daemon"
)

// validateConfig validates the configuration.
func validateConfig(cfg *config.Config) error {
	if len(cfg.RelayCallsign) > 9 {
		return fmt.Errorf("relay callsign must be 9 characters or less")
	}
	if cfg.BindAddress == "" {
		return fmt.Errorf("bind address cannot be empty")
	}
	if !isValidIPAddress(cfg.BindAddress) {
		return fmt.Errorf("invalid bind address format")
	}
	if cfg.WebInterfaceAddress == "" {
		return fmt.Errorf("web interface address cannot be empty")
	}
	if !isValidIPAddress(cfg.WebInterfaceAddress) {
		return fmt.Errorf("invalid web interface address format")
	}
	for _, relay := range cfg.TargetRelays {
		if !isValidIPAddress(relay.Address) {
			return fmt.Errorf("invalid target relay address: %s", relay.Address)
		}
	}
	return nil
}

// isValidIPAddress validates the IP address format.
func isValidIPAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil
}

// startServices starts the relay services.
func startServices(ctx context.Context, r *relay.Relay, wg *sync.WaitGroup, cfg *config.Config, mc *metrics.MetricsCollector) {
	services := []struct {
		name string
		fn   func(context.Context, *sync.WaitGroup)
	}{
		{"Listen", r.Listen},
		{"ConnectToRelays", r.ConnectToRelays},
		{"PingPeers", r.PingPeers},
		{"RemoveInactivePeers", r.RemoveInactivePeers},
		{"LogPeerState", r.LogPeerState},
	}

	for _, svc := range services {
		wg.Add(1)
		go func(name string, fn func(context.Context, *sync.WaitGroup)) {
			logging.LogInfo(fmt.Sprintf("Starting %s service", name), nil)
			fn(ctx, wg)
		}(svc.name, svc.fn)
	}

	// Initialize and start the HTTP server
	wg.Add(1)
	go mc.InitializeHTTPServer(ctx, wg, cfg)
}

func main() {
	flag.Parse()

	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		logging.LogError("Failed to load configuration", map[string]interface{}{"error": err})
		os.Exit(1)
	}

	if err := validateConfig(cfg); err != nil {
		logging.LogError("Invalid configuration", map[string]interface{}{"error": err})
		os.Exit(1)
	}

	logging.InitLogLevel(cfg.LogLevel)

	if cfg.DaemonMode {
		cntxt := &daemon.Context{
			PidFileName: "m17-relay.pid",
			PidFilePerm: 0644,
			LogFileName: "m17-relay.log",
			LogFilePerm: 0640,
			WorkDir:     "./",
			Umask:       027,
			Args:        []string{"[m17-relay]"},
		}

		d, err := cntxt.Reborn()
		if err != nil {
			logging.LogError("Failed to daemonize", map[string]interface{}{"error": err})
			os.Exit(1)
		}
		if d != nil {
			return
		}
		defer cntxt.Release()
		logging.LogInfo("Daemon started", nil)
	}

	mc := metrics.NewMetricsCollector(cfg.RelayCallsign)
	r := relay.NewRelay(cfg.BindAddress, cfg.RelayCallsign, cfg.TargetRelays, mc)
	if r == nil {
		logging.LogError("Failed to start relay", nil)
		os.Exit(1)
	}
	defer r.Close()

	logging.LogInfo("Relay initialized", map[string]interface{}{
		"callsign": cfg.RelayCallsign,
		"targets":  cfg.TargetRelays,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	var wg sync.WaitGroup

	startServices(ctx, r, &wg, cfg, mc)

	<-stop
	logging.LogInfo("Received shutdown signal", nil)
	cancel()

	shutdownComplete := make(chan struct{})
	go func() {
		wg.Wait()
		close(shutdownComplete)
	}()

	select {
	case <-shutdownComplete:
		logging.LogInfo("Graceful shutdown complete", nil)
	case <-time.After(10 * time.Second):
		logging.LogError("Shutdown timed out", nil)
	}
}
