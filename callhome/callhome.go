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

package callhome

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-m17-relay/config"
	"go-m17-relay/logging"
)

func getExternalIP() (string, error) {
	resp, err := http.Get("https://icanhazip.com")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

func CallHome(ctx context.Context, cfg *config.Config) {
	if !cfg.CallHomeEnabled {
		return
	}

	externalIP, err := getExternalIP()
	if err != nil {
		logging.LogError("Failed to get external IP address", map[string]interface{}{"error": err})
		return
	}

	_, portStr, err := net.SplitHostPort(cfg.BindAddress)
	if err != nil {
		logging.LogError("Failed to parse bind address", map[string]interface{}{"error": err})
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		logging.LogError("Failed to convert port to integer", map[string]interface{}{"error": err})
		return
	}

	data := map[string]interface{}{
		"uuid":       cfg.UUID,
		"callsign":   cfg.RelayCallsign,
		"ip_address": externalIP,
		"udp_port":   port,
		"status":     true,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		logging.LogError("Failed to marshal call-home data", map[string]interface{}{"error": err})
		return
	}

	// Log the request payload
	logging.LogInfo("Request payload", map[string]interface{}{
		"payload": string(jsonData),
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://relay.m17.link/api/v1/register", bytes.NewBuffer(jsonData))
	if err != nil {
		logging.LogError("Failed to create call-home request", map[string]interface{}{"error": err})
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Relay-Program", "go-m17-relay_v0.0.1")

	// Log the headers to verify they are set correctly
	logging.LogInfo("Request headers", map[string]interface{}{
		"Content-Type":    req.Header.Get("Content-Type"),
		"X-Relay-Program": req.Header.Get("X-Relay-Program"),
	})

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logging.LogError("Failed to send call-home request", map[string]interface{}{"error": err})
		return
	}
	defer resp.Body.Close()

	// Log the response status and body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.LogError("Failed to read response body", map[string]interface{}{"error": err})
		return
	}

	logging.LogInfo("Response status", map[string]interface{}{
		"status_code": resp.StatusCode,
	})
	logging.LogInfo("Response body", map[string]interface{}{
		"body": string(respBody),
	})

	if resp.StatusCode != http.StatusOK {
		logging.LogError("Call-home request failed", map[string]interface{}{"status_code": resp.StatusCode})
		return
	}

	var response map[string]interface{}
	if err := json.NewDecoder(bytes.NewBuffer(respBody)).Decode(&response); err != nil {
		logging.LogError("Failed to decode call-home response", map[string]interface{}{"error": err})
		return
	}

	if newUUID, ok := response["uuid"].(string); ok && cfg.UUID == "" {
		cfg.UUID = newUUID
		if err := saveConfig(cfg); err != nil {
			logging.LogError("Failed to save new UUID to config", map[string]interface{}{"error": err})
		}
	}

	logging.LogInfo("Call-home request succeeded", nil)
}

func saveConfig(cfg *config.Config) error {
	file, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile("config.json", file, 0644)
}

func StartPeriodicCallHome(ctx context.Context, wg *sync.WaitGroup, cfg *config.Config) {
	defer wg.Done()

	// Run CallHome once on startup
	logging.LogInfo("CallHome: Enabled, initial update on service start", nil)
	CallHome(ctx, cfg)

	ticker := time.NewTicker(7*time.Minute + 30*time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			randomDelay := time.Duration(rand.Intn(7*60+30)) * time.Second
			logging.LogInfo("CallHome: Periodic update with random delay", map[string]interface{}{
				"random_delay_seconds": randomDelay.Seconds(),
			})
			time.Sleep(randomDelay)
			CallHome(ctx, cfg)
		case <-ctx.Done():
			logging.LogInfo("CallHome: Shutdown signal received, stopping...", nil)
			return
		}
	}
}
