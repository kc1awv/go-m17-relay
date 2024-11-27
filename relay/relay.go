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

package relay

import (
	"context"
	"go-m17-relay/logging"
	"net"
	"sync"
	"time"
)

// WorkerPoolSize defines the number of workers for pinging clients
// concurrently.
const (
	WorkerPoolSize = 5
)

// ClientState holds information about a client, such as their last received
// PONG and callsign.
type ClientState struct {
	LastPong time.Time
	Callsign string
}

// Relay represents the M17 relay server, handling client connections and
// communication.
type Relay struct {
	Clients       sync.Map     // Holds all currently connected clients, keyed by their IP address.
	ClientsLock   sync.Mutex   // Mutex for managing access to the Clients map.
	Socket        *net.UDPConn // UDP socket used for communication with clients.
	RelayCallsign string       // Callsign of the relay itself.
}

// Constants for packet types and sizes.
const (
	PacketSize   = 10
	MagicConn    = "CONN"
	MagicAckn    = "ACKN"
	MagicNack    = "NACK"
	MagicPing    = "PING"
	MagicPong    = "PONG"
	MagicDisc    = "DISC"
	CallsignSize = 6
)

// NewRelay initializes a new Relay object, listening on the given address.
func NewRelay(addr string, callsign string) *Relay {
	// Resolve the UDP address.
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		logging.LogError("Failed to resolve address:", map[string]interface{}{"err": err})
		return nil
	}

	// Listen for incoming UDP packets.
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		logging.LogError("Failed to listen on:", map[string]interface{}{"addr": addr, "err": err})
	}

	logging.LogInfo("Relay is listening on:", map[string]interface{}{"addr": addr})

	// Return a new Relay object with the given parameters.
	return &Relay{
		Clients:       sync.Map{},
		ClientsLock:   sync.Mutex{},
		Socket:        conn,
		RelayCallsign: callsign,
	}
}

// Listen starts receiving packets and processes them based on the packet type
// (data or control).
func (r *Relay) Listen(ctx context.Context) {
	buf := make([]byte, 64)

	logging.LogDebug("Relay is ready to receive packets...", nil)

	for {
		select {
		case <-ctx.Done():
			logging.LogInfo("Received shutdown signal, stopping listener...", nil)
			return
		default:
			n, addr, err := r.Socket.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(*net.OpError); ok && ne.Op == "read" && ne.Err.Error() == "use of closed network connection" {
					logging.LogDebug("Socket closed, exiting listen loop.", nil)
					return
				}
				logging.LogError("Error reading packet", map[string]interface{}{"err": err})
				continue
			}

			logging.LogDebug("Received packet", map[string]interface{}{"from": addr, "packet": buf[:n]})

			// Determine if the packet is a data packet or control packet.
			if n >= 4 && string(buf[:4]) == "M17 " {
				r.relayDataPacket(buf[:n], addr)
			} else {
				r.handleControlPacket(buf[:n], addr)
			}
		}
	}
}

// handleControlPacket processes incoming control packets (CONN, PONG, DISC, etc.).
func (r *Relay) handleControlPacket(data []byte, addr *net.UDPAddr) {
	if len(data) < 4 {
		logging.LogDebug("Packet too short", map[string]interface{}{"from": addr.String()})
		return
	}

	magic := string(data[:4])
	switch magic {
	case MagicConn:
		if len(data) < PacketSize {
			logging.LogDebug("Invalid CONN packet length", map[string]interface{}{"from": addr.String()})
			return
		}
		callsign := DecodeCallsign(data[4:10])
		module := byte(0)
		if len(data) > 10 {
			module = data[10]
		}
		r.handleConnPacket(callsign, addr, module)
	case MagicPong:
		if len(data) < PacketSize {
			logging.LogDebug("Invalid PONG packet length", map[string]interface{}{"from": addr.String()})
			return
		}
		callsign := DecodeCallsign(data[4:10])
		r.handlePongPacket(callsign, addr)
	case MagicDisc:
		if len(data) < PacketSize {
			logging.LogDebug("Invalid DISC packet length", map[string]interface{}{"from": addr.String()})
			return
		}
		callsign := DecodeCallsign(data[4:10])
		r.handleDiscPacket(callsign, addr)
	default:
		logging.LogDebug("Unknown packet type from", map[string]interface{}{"from": addr.String()})
	}
}

// handleConnPacket processes a connection request (CONN packet).
func (r *Relay) handleConnPacket(callsign string, addr *net.UDPAddr, module byte) {
	if module != 0 {
		logging.LogDebug("Received connection request with module (We don't need no stinkin' modules.)",
			map[string]interface{}{"from": addr.String(), "module": module})
	} else {
		logging.LogDebug("Received connection request with no module. (Yay, relay!)",
			map[string]interface{}{"from": addr.String()})
	}

	// Check if the client is already connected.
	if _, exists := r.Clients.Load(addr.String()); exists {
		logging.LogInfo("Client is already connected. Sending NACK.", map[string]interface{}{"from": addr.String()})
		r.sendPacket(MagicNack, addr, nil)
	} else {
		logging.LogInfo("Accepting connection. Sending ACKN.",
			map[string]interface{}{"from": addr.String(), "callsign": callsign})
		r.Clients.Store(addr.String(), &ClientState{
			LastPong: time.Now(),
			Callsign: callsign,
		})
		r.sendPacket(MagicAckn, addr, nil)
	}
}

// handleDiscPacket processes a DISCONNECT (DISC) packet and removes the client.
func (r *Relay) handlePongPacket(callsign string, addr *net.UDPAddr) {
	if client, exists := r.Clients.Load(addr.String()); exists {
		client.(*ClientState).LastPong = time.Now()
		logging.LogDebug("Received PONG", map[string]interface{}{"from": addr.String(), "callsign": callsign})
	} else {
		logging.LogDebug("PONG from unregistered client", map[string]interface{}{"from": addr.String(), "callsign": callsign})
	}
}

func (r *Relay) handleDiscPacket(callsign string, addr *net.UDPAddr) {
	if _, exists := r.Clients.Load(addr.String()); exists {
		r.Clients.Delete(addr.String())
		logging.LogInfo("Client disconnected.", map[string]interface{}{"from": addr.String(), "callsign": callsign})

		// Encode and send a DISC packet back to the client.
		encodedCallsign, err := EncodeCallsign(r.RelayCallsign)
		if err != nil {
			logging.LogError("Failed to encode relay callsign", map[string]interface{}{"err": err})
			return
		}

		r.sendPacket(MagicDisc, addr, encodedCallsign)
	} else {
		logging.LogDebug("DISC from unregistered client", map[string]interface{}{"from": addr.String(), "callsign": callsign})
	}
}

// sendPacket sends a control packet to the specified address.
func (r *Relay) sendPacket(magic string, addr *net.UDPAddr, callsign []byte) {
	packet := make([]byte, 4)
	copy(packet, []byte(magic))

	if callsign != nil {
		packet = append(packet, callsign...)
	}

	_, err := r.Socket.WriteToUDP(packet, addr)
	if err != nil {
		logging.LogError("Failed to send packet", map[string]interface{}{"to": addr.String(), "err": err})
	}
}

// sendPing sends a PING packet to the specified client.
func (r *Relay) sendPing(addr string) {
	encodedCallsign, err := EncodeCallsign(r.RelayCallsign)
	if err != nil {
		logging.LogError("Failed to encode relay callsign", map[string]interface{}{"err": err})
		return
	}

	packet := make([]byte, 10)
	copy(packet[:4], []byte(MagicPing))
	copy(packet[4:], encodedCallsign)

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		logging.LogError("Failed to resolve client address", map[string]interface{}{"addr": addr, "err": err})
		return
	}

	_, err = r.Socket.WriteToUDP(packet, udpAddr)
	if err != nil {
		logging.LogError("Failed to send PING", map[string]interface{}{"to": addr, "err": err})
	}
}

// pingWorker is responsible for sending PING packets to clients in parallel.
func (r *Relay) pingWorker(tasks <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for addr := range tasks {
		r.sendPing(addr)
	}
}

func (r *Relay) PingClients(ctx context.Context) {
	tasks := make(chan string, 100) // You can adjust the channel size based on expected number of clients
	var wg sync.WaitGroup

	for i := 0; i < WorkerPoolSize; i++ {
		go r.pingWorker(tasks, &wg)
	}

	for {
		select {
		case <-ctx.Done():
			logging.LogInfo("PingClients: Shutdown signal received, stopping...", nil)
			close(tasks)
			wg.Wait()
			return
		case <-time.After(1 * time.Second):
			r.Clients.Range(func(key, value interface{}) bool {
				addr := key.(string)
				tasks <- addr
				return true
			})
		}
	}
}

// RemoveInactiveClients removes clients that have not sent a PONG packet to
// the relay and may be lost
func (r *Relay) RemoveInactiveClients(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logging.LogInfo("RemoveInactiveClients: Shutdown signal received, stopping...", nil)
			return
		case <-time.After(1 * time.Second):
			now := time.Now()
			r.ClientsLock.Lock()
			r.Clients.Range(func(key, value interface{}) bool {
				client := value.(*ClientState)
				if now.Sub(client.LastPong) > 30*time.Second {
					logging.LogInfo("Removing inactive client", map[string]interface{}{"client": key})
					r.Clients.Delete(key)
				}
				return true
			})
			r.ClientsLock.Unlock()
		}
	}
}

// logClientState prints a log that shows the number of clients connected and
// their details
func (r *Relay) LogClientState() {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	for range tick.C {
		// Log the number of currently connected clients
		numClients := 0
		r.Clients.Range(func(key, value interface{}) bool {
			numClients++
			return true
		})
		logging.LogDebug("Current clients:", map[string]interface{}{
			"count": numClients,
		})

		// Log each client's information
		r.Clients.Range(func(key, value interface{}) bool {
			client := value.(*ClientState)
			logging.LogDebug("Client",
				map[string]interface{}{
					"key":      key, // Client identifier (e.g., IP address)
					"callsign": client.Callsign,
					"lastPong": client.LastPong.Format(time.RFC3339),
				},
			)
			return true
		})
	}
}

// relayDataPacket handles data packets from clients (to be processed in the future).
func (r *Relay) relayDataPacket(packet []byte, senderAddr *net.UDPAddr) {
	logging.LogDebug("Relaying data packet to other clients", map[string]interface{}{"from": senderAddr})

	r.Clients.Range(func(key, value interface{}) bool {
		addr := key.(string)
		if addr != senderAddr.String() {
			logging.LogDebug("Forwarding data packet", map[string]interface{}{"to": addr})
			udpAddr, err := net.ResolveUDPAddr("udp", addr)
			if err != nil {
				logging.LogError("Error resolving address", map[string]interface{}{"addr": addr, "err": err})
				return true
			}

			_, err = r.Socket.WriteToUDP(packet, udpAddr)
			if err != nil {
				logging.LogError("Error forwarding data packet to %s: %v\n", map[string]interface{}{"addr": addr, "err": err})
			}
		}
		return true
	})
}
