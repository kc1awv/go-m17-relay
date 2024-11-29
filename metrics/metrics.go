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

package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Metrics holds the metrics for the relay server.
type Metrics struct {
	NumClients int        `json:"num_clients"`
	NumRelays  int        `json:"num_relays"`
	Clients    []PeerInfo `json:"clients"`
	Relays     []PeerInfo `json:"relays"`
}

// GetPeerInfo returns the current clients and relays.
func (mc *MetricsCollector) GetPeerInfo() ([]PeerInfo, []PeerInfo) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	clients := make([]PeerInfo, 0, len(mc.clients))
	for _, info := range mc.clients {
		clients = append(clients, info)
	}

	relays := make([]PeerInfo, 0, len(mc.relays))
	for _, info := range mc.relays {
		relays = append(relays, info)
	}

	return clients, relays
}

// PeerInfo holds information about a peer (client or relay).
type PeerInfo struct {
	Callsign       string    `json:"callsign"`
	LastPong       time.Time `json:"last_pong"`
	ConnectedAt    time.Time `json:"connected_at"`
	LastDataPacket time.Time `json:"last_data_packet"`
}

// MetricsCollector collects and serves metrics.
type MetricsCollector struct {
	clients       map[string]PeerInfo
	relays        map[string]PeerInfo
	relayCallsign string
	mu            sync.RWMutex
}

// NewMetricsCollector creates a new MetricsCollector.
func NewMetricsCollector(relayCallsign string) *MetricsCollector {
	return &MetricsCollector{
		clients:       make(map[string]PeerInfo),
		relays:        make(map[string]PeerInfo),
		relayCallsign: relayCallsign,
	}
}

// UpdateMetrics updates the metrics.
func (mc *MetricsCollector) UpdateMetrics(clients, relays map[string]PeerInfo) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.clients = clients
	mc.relays = relays
}

// GetMetrics returns the current metrics.
func (mc *MetricsCollector) GetMetrics() Metrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	clients := make([]PeerInfo, 0, len(mc.clients))
	for _, info := range mc.clients {
		clients = append(clients, info)
	}

	relays := make([]PeerInfo, 0, len(mc.relays))
	for _, info := range mc.relays {
		relays = append(relays, info)
	}

	sort.Slice(clients, func(i, j int) bool {
		return clients[i].Callsign < clients[j].Callsign
	})
	sort.Slice(relays, func(i, j int) bool {
		return relays[i].Callsign < relays[j].Callsign
	})

	return Metrics{
		NumClients: len(mc.clients),
		NumRelays:  len(mc.relays),
		Clients:    clients,
		Relays:     relays,
	}
}

// ServeMetrics serves the metrics as a JSON response.
func (mc *MetricsCollector) ServeMetrics(w http.ResponseWriter, req *http.Request) {
	metrics := mc.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// ServeWebInterface serves a simple web interface to display the metrics.
func (mc *MetricsCollector) ServeWebInterface(w http.ResponseWriter, req *http.Request) {
	clients, relays := mc.GetPeerInfo()
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`
        <!DOCTYPE html>
        <html>
        <head>
            <title>Relay Metrics - ` + mc.relayCallsign + `</title>
            <style>
                body {
                    font-family: Arial, sans-serif;
                    margin: 0;
                    padding: 0;
                    background-color: #f4f4f4;
                }
                .container {
                    width: 80%;
                    margin: auto;
                    overflow: hidden;
                }
                header {
                    background: #333;
                    color: #fff;
                    padding-top: 30px;
                    min-height: 70px;
                    border-bottom: #77aaff 3px solid;
                }
                header h1 {
                    text-align: center;
                    text-transform: uppercase;
                    margin: 0;
                    font-size: 24px;
                }
                .metrics-box {
                    display: flex;
                    justify-content: space-around;
                    margin: 20px 0;
                }
                .metrics-box div {
                    background: #fff;
                    padding: 20px;
                    border: 1px solid #ddd;
                    border-radius: 5px;
                    width: 45%;
                    text-align: center;
                    box-shadow: 0 0 10px rgba(0, 0, 0, 0.1);
                }
                table {
                    width: 100%;
                    margin: 20px 0;
                    border-collapse: collapse;
                }
                table, th, td {
                    border: 1px solid #ddd;
                }
                th, td {
                    padding: 12px;
                    text-align: left;
                }
                th {
                    background-color: #333;
                    color: white;
                }
                tr:nth-child(even) {
                    background-color: #f2f2f2;
                }
            </style>
            <script>
                function fetchMetrics() {
                    fetch('/metrics')
                        .then(response => response.json())
                        .then(data => {
                            document.getElementById('numClients').innerText = data.num_clients;
                            document.getElementById('numRelays').innerText = data.num_relays;
                            updateTable('clientsTable', data.clients);
                            updateTable('relaysTable', data.relays);
                        })
                        .catch(error => console.error('Error fetching metrics:', error));
                }

                function updateTable(tableId, data) {
                    const table = document.getElementById(tableId);
                    table.innerHTML = '';
                    data.forEach(item => {
                        const row = table.insertRow();
                        row.insertCell(0).innerText = item.callsign;
                        row.insertCell(1).innerText = item.connected_at ? formatDateTime(item.connected_at) : 'N/A';
                        row.insertCell(2).innerText = item.last_pong ? formatDateTime(item.last_pong) : 'N/A';
                        row.insertCell(3).innerText = item.last_data_packet && item.last_data_packet !== "0001-01-01T00:00:00Z" ? formatDateTime(item.last_data_packet) : 'No data received';
                    });
                }

                function formatDateTime(dateTime) {
                    const options = { year: 'numeric', month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' };
                    return new Date(dateTime).toLocaleString('en-US', options);
                }

                setInterval(fetchMetrics, 5000);
                window.onload = fetchMetrics;
            </script>
        </head>
        <body>
            <header>
                <h1>Relay Metrics - ` + mc.relayCallsign + `</h1>
            </header>
            <div class="container">
                <div class="metrics-box">
                    <div>
                        <h2>Number of Clients</h2>
                        <p id="numClients">` + fmt.Sprintf("%d", len(clients)) + `</p>
                    </div>
                    <div>
                        <h2>Number of Relays</h2>
                        <p id="numRelays">` + fmt.Sprintf("%d", len(relays)) + `</p>
                    </div>
                </div>
                <h2>Clients</h2>
                <table>
                    <tr>
                        <th>Callsign</th>
                        <th>Connected At</th>
                        <th>Last PONG</th>
                        <th>Last Data Packet</th>
                    </tr>
                    <tbody id="clientsTable">`))
	for _, client := range clients {
		lastDataPacket := "No data received"
		if !client.LastDataPacket.IsZero() {
			lastDataPacket = client.LastDataPacket.Format("02 Jan 2006 15:04:05")
		}
		w.Write([]byte(`
                    <tr>
                        <td>` + client.Callsign + `</td>
                        <td>` + client.ConnectedAt.Format("02 Jan 2006 15:04:05") + `</td>
                        <td>` + client.LastPong.Format("02 Jan 2006 15:04:05") + `</td>
                        <td>` + lastDataPacket + `</td>
                    </tr>`))
	}
	w.Write([]byte(`
                    </tbody>
                </table>
                <h2>Relays</h2>
                <table>
                    <tr>
                        <th>Callsign</th>
                        <th>Connected At</th>
                        <th>Last PONG</th>
                        <th>Last Data Packet</th>
                    </tr>
                    <tbody id="relaysTable">`))
	for _, relay := range relays {
		lastDataPacket := "No data received"
		if !relay.LastDataPacket.IsZero() {
			lastDataPacket = relay.LastDataPacket.Format("02 Jan 2006 15:04:05")
		}
		w.Write([]byte(`
                    <tr>
                        <td>` + relay.Callsign + `</td>
                        <td>` + relay.ConnectedAt.Format("02 Jan 2006 15:04:05") + `</td>
                        <td>` + relay.LastPong.Format("02 Jan 2006 15:04:05") + `</td>
                        <td>` + lastDataPacket + `</td>
                    </tr>`))
	}
	w.Write([]byte(`
                    </tbody>
                </table>
            </div>
        </body>
        </html>
    `))
}
