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
	"fmt"
	"go-m17-relay/config"
	"go-m17-relay/logging"
	"go-m17-relay/relay"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func validateConfig(cfg *config.Config) error {
	if len(cfg.RelayCallsign) > 9 {
		return fmt.Errorf("relay callsign must be 9 characters or less")
	}
	if cfg.BindAddress == "" {
		return fmt.Errorf("bind address cannot be empty")
	}
	return nil
}

func startServices(ctx context.Context, r *relay.Relay, wg *sync.WaitGroup) {
	services := []struct {
		name string
		fn   func(context.Context, *sync.WaitGroup)
	}{
		{"Listen", r.Listen},
		{"ConnectToRelays", r.ConnectToRelays},
		{"PingClients", r.PingClients},
		{"RemoveInactiveClients", r.RemoveInactiveClients},
		{"LogClientState", r.LogClientState},
	}

	for _, svc := range services {
		wg.Add(1)
		go func(name string, fn func(context.Context, *sync.WaitGroup)) {
			logging.LogInfo(fmt.Sprintf("Starting %s service", name), nil)
			fn(ctx, wg)
		}(svc.name, svc.fn)
	}
}

func main() {
	// Load configuration
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

	// Initialize relay
	r := relay.NewRelay(cfg.BindAddress, cfg.RelayCallsign, cfg.TargetRelays)
	if r == nil {
		logging.LogError("Failed to start relay", nil)
		os.Exit(1)
	}
	defer r.Close()

	logging.LogInfo("Relay initialized", map[string]interface{}{
		"callsign": cfg.RelayCallsign,
		"targets":  cfg.TargetRelays,
	})

	// Setup shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Start services
	var wg sync.WaitGroup
	startServices(ctx, r, &wg)

	// Wait for shutdown signal
	<-stop
	logging.LogInfo("Received shutdown signal", nil)
	cancel()

	// Wait for graceful shutdown with timeout
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
