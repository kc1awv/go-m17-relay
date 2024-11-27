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
	"go-m17-relay/config"
	"go-m17-relay/logging"
	"go-m17-relay/relay"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Load the configuration
	config, err := config.LoadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	logging.InitLogLevel(config.LogLevel)

	relayCallsign := config.RelayCallsign
	if len(relayCallsign) > 9 {
		log.Fatalf("Relay callsign must be 9 characters or less.")
	}

	logging.LogInfo("Relay callsign", map[string]interface{}{"relayCallsign": relayCallsign})

	// Initialize the relay
	relay := relay.NewRelay(config.BindAddress, relayCallsign)
	if relay == nil {
		logging.LogError("Failed to start relay.", nil)
	}

	// Create a context to signal shutdown to the goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a signal channel to catch SIGINT or SIGTERM (for graceful shutdown)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Start the relay tasks in separate goroutines
	go relay.Listen(ctx)
	go relay.PingClients(ctx)
	go relay.RemoveInactiveClients(ctx)
	go relay.LogClientState()

	// Wait for shutdown signal
	<-stop
	logging.LogInfo("Received shutdown signal, shutting down...", nil)

	// Cancel the context to stop the relay tasks
	cancel()

	// Wait for the relay to clean up and stop
	logging.LogInfo("Relay shutdown complete.", nil)
}
