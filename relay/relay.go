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
	"go-m17-relay/config"
	"go-m17-relay/logging"
	"go-m17-relay/metrics"
	"net"
	"sync"
	"time"
)

const (
	WorkerPoolSize = 5
)

type ClientState struct {
	ConnectedAt    time.Time
	LastPong       time.Time
	LastDataPacket time.Time
	Callsign       string
	ListenOnly     bool
}

type LinkState struct {
	ConnectedAt    time.Time
	LastPong       time.Time
	LastDataPacket time.Time
	Callsign       string
}

type Relay struct {
	Clients          sync.Map
	LinkedRelays     sync.Map
	TargetRelays     map[string]string
	ClientsLock      sync.Mutex
	Socket           *net.UDPConn
	RelayCallsign    string
	MetricsCollector *metrics.MetricsCollector
	StartTime        time.Time
}

const (
	PacketSize   = 10
	MagicConn    = "CONN"
	MagicLink    = "LINK"
	MagicLstn    = "LSTN"
	MagicAckn    = "ACKN"
	MagicNack    = "NACK"
	MagicPing    = "PING"
	MagicPong    = "PONG"
	MagicInfo    = "INFO"
	MagicDisc    = "DISC"
	CallsignSize = 6
)

// NewRelay creates a new Relay instance.
func NewRelay(addr string, callsign string, targetRelays []config.TargetRelay, mc *metrics.MetricsCollector) *Relay {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		logging.LogError("Failed to resolve address:", map[string]interface{}{"err": err})
		return nil
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		logging.LogError("Failed to listen on:", map[string]interface{}{"addr": addr, "err": err})
	}

	logging.LogInfo("Relay is listening on:", map[string]interface{}{"addr": addr})

	targetRelaysMap := make(map[string]string)
	for _, relay := range targetRelays {
		targetRelaysMap[relay.Callsign] = relay.Address
	}

	return &Relay{
		Clients:          sync.Map{},
		ClientsLock:      sync.Mutex{},
		Socket:           conn,
		RelayCallsign:    callsign,
		TargetRelays:     targetRelaysMap,
		MetricsCollector: mc,
		StartTime:        time.Now(),
	}
}

// Listen listens for incoming packets and processes them.
func (r *Relay) Listen(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 64)

	logging.LogDebug("Relay is ready to receive packets...", nil)

	for {
		select {
		case <-ctx.Done():
			logging.LogInfo("Listen: Shutdown signal received, stopping...", nil)
			return
		default:
			r.Socket.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, addr, err := r.Socket.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(*net.OpError); ok && ne.Timeout() {
					continue
				}
				if ne, ok := err.(*net.OpError); ok && ne.Op == "read" && ne.Err.Error() == "use of closed network connection" {
					logging.LogDebug("Socket closed, exiting listen loop.", nil)
					return
				}
				logging.LogError("Error reading packet", map[string]interface{}{"err": err})
				continue
			}

			logging.LogDebug("Received packet", map[string]interface{}{"from": addr, "packet": buf[:n]})

			if n >= 4 && string(buf[:4]) == "M17 " {
				r.relayDataPacket(buf[:n], addr)
			} else {
				r.handleControlPacket(buf[:n], addr)
			}
		}
	}
}

// UpdateMetrics updates the metrics collector with the current state of connected clients and relays.
func (r *Relay) UpdateMetrics() {
	clients := make(map[string]metrics.PeerInfo)
	relays := make(map[string]metrics.PeerInfo)
	r.Clients.Range(func(key, value interface{}) bool {
		client := value.(*ClientState)
		clients[key.(string)] = metrics.PeerInfo{
			Callsign:       client.Callsign,
			LastPong:       client.LastPong,
			ConnectedAt:    client.ConnectedAt,
			LastDataPacket: client.LastDataPacket,
		}
		return true
	})
	r.LinkedRelays.Range(func(key, value interface{}) bool {
		relay := value.(*LinkState)
		relays[key.(string)] = metrics.PeerInfo{
			Callsign:       relay.Callsign,
			LastPong:       relay.LastPong,
			ConnectedAt:    relay.ConnectedAt,
			LastDataPacket: relay.LastDataPacket,
		}
		return true
	})
	r.MetricsCollector.UpdateMetrics(clients, relays)
}

// handleControlPacket processes a control packet.
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
	case MagicLink:
		if len(data) < PacketSize {
			logging.LogDebug("Invalid LINK packet length", map[string]interface{}{"from": addr.String()})
			return
		}
		callsign := DecodeCallsign(data[4:10])
		r.handleLinkPacket(callsign, addr)
	case MagicLstn:
		if len(data) < PacketSize {
			logging.LogDebug("Invalid LSTN packet length", map[string]interface{}{"from": addr.String()})
			return
		}
		callsign := DecodeCallsign(data[4:10])
		r.handleLstnPacket(callsign, addr)
	case MagicAckn:
		if len(data) < 4 {
			logging.LogDebug("Invalid ACKN packet length", map[string]interface{}{"from": addr.String()})
			return
		}
		r.handleAcknPacket(addr)
	case MagicPing:
		if len(data) < PacketSize {
			logging.LogDebug("Invalid PING packet length", map[string]interface{}{"from": addr.String()})
			return
		}
		callsign := DecodeCallsign(data[4:10])
		r.handlePingPacket(callsign, addr)
	case MagicPong:
		if len(data) < PacketSize {
			logging.LogDebug("Invalid PONG packet length", map[string]interface{}{"from": addr.String()})
			return
		}
		callsign := DecodeCallsign(data[4:10])
		r.handlePongPacket(callsign, addr)
	case MagicInfo:
		if len(data) < 5 || data[4] != '?' {
			logging.LogDebug("Invalid INFO? packet", map[string]interface{}{"from": addr.String()})
			return
		}
		r.handleInfoPacket(addr)
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

	if _, exists := r.Clients.Load(addr.String()); exists {
		logging.LogInfo("Client is already connected. Sending NACK.", map[string]interface{}{"from": addr.String()})
		r.sendPacket(MagicNack, addr, nil)
	} else {
		logging.LogInfo("Accepting connection. Sending ACKN.",
			map[string]interface{}{"from": addr.String(), "callsign": callsign})
		r.Clients.Store(addr.String(), &ClientState{
			ConnectedAt: time.Now(),
			LastPong:    time.Now(),
			Callsign:    callsign,
			ListenOnly:  false,
		})
		r.UpdateMetrics()
		r.sendPacket(MagicAckn, addr, nil)
	}
}

// handleLinkPacket processes a link request (LINK packet).
func (r *Relay) handleLinkPacket(callsign string, addr *net.UDPAddr) {
	if _, exists := r.LinkedRelays.Load(addr.String()); exists {
		logging.LogInfo("Relay is already linked", map[string]interface{}{"from": addr.String()})
		return
	}

	expectedAddr, exists := r.TargetRelays[callsign]
	if !exists || expectedAddr != addr.String() {
		logging.LogInfo("Invalid LINK packet", map[string]interface{}{"from": addr.String(), "callsign": callsign})
		r.sendPacket(MagicNack, addr, nil)
		return
	}

	logging.LogInfo("Establishing link with relay", map[string]interface{}{"from": addr.String(), "callsign": callsign})

	r.LinkedRelays.Store(addr.String(), &LinkState{
		ConnectedAt: time.Now(),
		LastPong:    time.Now(),
		Callsign:    callsign,
	})

	r.UpdateMetrics()
	r.sendPacket(MagicAckn, addr, nil)
}

// handleLstnPacket processes a listen-only connection request (LSTN packet).
func (r *Relay) handleLstnPacket(callsign string, addr *net.UDPAddr) {
	logging.LogDebug("Received listen-only connection request", map[string]interface{}{"from": addr.String(), "callsign": callsign})
	if _, exists := r.Clients.Load(addr.String()); exists {
		logging.LogInfo("Client is already connected. Sending NACK.", map[string]interface{}{"from": addr.String()})
		r.sendPacket(MagicNack, addr, nil)
	} else {
		logging.LogInfo("Accepting listen-only connection. Sending ACKN.",
			map[string]interface{}{"from": addr.String(), "callsign": callsign})
		r.Clients.Store(addr.String(), &ClientState{
			ConnectedAt: time.Now(),
			LastPong:    time.Now(),
			Callsign:    callsign,
			ListenOnly:  true,
		})
		r.UpdateMetrics()
		r.sendPacket(MagicAckn, addr, nil)
	}
}

// handleAcknPacket processes an ACKN packet, which is sent by a relay to acknowledge a link request.
func (r *Relay) handleAcknPacket(addr *net.UDPAddr) {
	logging.LogDebug("Received ACKN packet", map[string]interface{}{"from": addr.String()})
	if _, exists := r.LinkedRelays.Load(addr.String()); exists {
		logging.LogInfo("Relay is already linked", map[string]interface{}{"from": addr.String()})
		return
	}

	logging.LogInfo("Establishing link with relay", map[string]interface{}{"from": addr.String()})

	r.LinkedRelays.Store(addr.String(), &LinkState{
		ConnectedAt: time.Now(),
		LastPong:    time.Now(),
		Callsign:    r.RelayCallsign,
	})
	r.UpdateMetrics()
}

// handlePingPacket processes a PING packet.
func (r *Relay) handlePingPacket(callsign string, addr *net.UDPAddr) {
	logging.LogDebug("Received PING packet", map[string]interface{}{"from": addr.String(), "callsign": callsign})
	logging.LogDebug("Responding with PONG packet", map[string]interface{}{"to": addr.String(), "callsign": callsign})
	r.sendPong(addr, callsign)
}

// handlePongPacket processes a PONG packet.
func (r *Relay) handlePongPacket(callsign string, addr *net.UDPAddr) {
	if client, exists := r.Clients.Load(addr.String()); exists {
		client.(*ClientState).LastPong = time.Now()
		logging.LogDebug("Received PONG from client", map[string]interface{}{"from": addr.String(), "callsign": callsign})
		r.UpdateMetrics()
	} else if relay, exists := r.LinkedRelays.Load(addr.String()); exists {
		relay.(*LinkState).LastPong = time.Now()
		logging.LogDebug("Received PONG from relay", map[string]interface{}{"from": addr.String(), "callsign": callsign})
		r.UpdateMetrics()
	} else {
		logging.LogDebug("PONG from unregistered peer", map[string]interface{}{"from": addr.String(), "callsign": callsign})
	}
}

// handleInfoPacket processes an INFO? packet.
func (r *Relay) handleInfoPacket(addr *net.UDPAddr) {
	logging.LogDebug("Received INFO? packet", map[string]interface{}{"from": addr.String()})

	uptime := uint32(time.Since(r.StartTime).Seconds())
	numClients := uint16(0)
	numRelays := uint16(0)

	r.Clients.Range(func(_, _ interface{}) bool {
		numClients++
		return true
	})
	r.LinkedRelays.Range(func(_, _ interface{}) bool {
		numRelays++
		return true
	})

	encodedCallsign, err := EncodeCallsign(r.RelayCallsign)
	if err != nil {
		logging.LogError("Failed to encode relay callsign", map[string]interface{}{"err": err})
		return
	}

	packet := make([]byte, 18)
	copy(packet[:4], []byte(MagicInfo))
	copy(packet[4:10], encodedCallsign)
	copy(packet[10:14], uint32ToBytes(uptime))
	copy(packet[14:16], uint16ToBytes(numClients))
	copy(packet[16:18], uint16ToBytes(numRelays))

	_, err = r.Socket.WriteToUDP(packet, addr)
	if err != nil {
		logging.LogError("Failed to send INFO packet", map[string]interface{}{"to": addr.String(), "err": err})
	}
}

func uint32ToBytes(value uint32) []byte {
	return []byte{
		byte(value >> 24),
		byte(value >> 16),
		byte(value >> 8),
		byte(value),
	}
}

func uint16ToBytes(value uint16) []byte {
	return []byte{
		byte(value >> 8),
		byte(value),
	}
}

// handleDiscPacket processes a DISC packet.
func (r *Relay) handleDiscPacket(callsign string, addr *net.UDPAddr) {
	if _, exists := r.Clients.Load(addr.String()); exists {
		r.Clients.Delete(addr.String())
		logging.LogInfo("Client disconnected.", map[string]interface{}{"from": addr.String(), "callsign": callsign})
		r.UpdateMetrics()

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

// sendPacket sends a packet to the specified address.
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

// sendPing sends a PING packet to the specified address.
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

// sendPong sends a PONG packet to the specified address.
func (r *Relay) sendPong(addr *net.UDPAddr, callsign string) {
	encodedCallsign, err := EncodeCallsign(callsign)
	if err != nil {
		logging.LogError("Failed to encode callsign", map[string]interface{}{"err": err})
		return
	}

	packet := make([]byte, 10)
	copy(packet[:4], []byte(MagicPong))
	copy(packet[4:], encodedCallsign)

	_, err = r.Socket.WriteToUDP(packet, addr)
	if err != nil {
		logging.LogError("Failed to send PONG", map[string]interface{}{"to": addr.String(), "err": err})
	}
}

// pingWorker is a worker that sends PING packets to clients and relays.
func (r *Relay) pingWorker(ctx context.Context, tasks <-chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case addr, ok := <-tasks:
			if !ok {
				return
			}
			r.sendPing(addr)
		}
	}
}

// PingPeers sends PING packets to all connected clients and relays.
func (r *Relay) PingPeers(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	tasks := make(chan string, 100)
	var workerWg sync.WaitGroup

	for i := 0; i < WorkerPoolSize; i++ {
		workerWg.Add(1)
		go r.pingWorker(ctx, tasks, &workerWg)
	}

	for {
		select {
		case <-ctx.Done():
			logging.LogInfo("PingPeers: Shutdown signal received, stopping...", nil)
			close(tasks)
			workerWg.Wait()
			return
		case <-ticker.C:
			r.Clients.Range(func(key, value interface{}) bool {
				tasks <- key.(string)
				return true
			})
			r.LinkedRelays.Range(func(key, value interface{}) bool {
				tasks <- key.(string)
				return true
			})
		}
	}
}

// RemoveInactivePeers removes clients and relays that have not sent a PONG packet in 30 seconds.
func (r *Relay) RemoveInactivePeers(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			logging.LogInfo("RemoveInactivePeers: Shutdown signal received, stopping...", nil)
			return
		case <-time.After(10 * time.Second):
			now := time.Now()
			r.ClientsLock.Lock()
			r.Clients.Range(func(key, value interface{}) bool {
				client := value.(*ClientState)
				if now.Sub(client.LastPong) > 30*time.Second {
					logging.LogInfo("Removing inactive client", map[string]interface{}{"client": key})
					r.Clients.Delete(key)
					r.UpdateMetrics()
				}
				return true
			})
			r.LinkedRelays.Range(func(key, value interface{}) bool {
				relay := value.(*LinkState)
				if now.Sub(relay.LastPong) > 30*time.Second {
					logging.LogInfo("Removing inactive linked relay", map[string]interface{}{"relay": key})
					r.LinkedRelays.Delete(key)
					r.UpdateMetrics()
				}
				return true
			})
			r.ClientsLock.Unlock()
		}
	}
}

// ConnectToRelays attempts to connect to the target relays.
func (r *Relay) ConnectToRelays(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	var workerWg sync.WaitGroup

	for _, target := range r.TargetRelays {
		workerWg.Add(1)
		go func(target string) {
			defer workerWg.Done()
			for {
				select {
				case <-ctx.Done():
					logging.LogInfo("ConnectToRelays: Shutdown signal received, stopping connection attempts...", map[string]interface{}{"target": target})
					return
				default:
					if _, exists := r.LinkedRelays.Load(target); exists {
						time.Sleep(10 * time.Second)
						continue
					}

					logging.LogInfo("Attempting to link to relay", map[string]interface{}{"target": target})
					err := r.sendLinkPacket(target)
					if err != nil {
						logging.LogError("Failed to send LINK packet", map[string]interface{}{"target": target, "err": err})
						time.Sleep(5 * time.Second)
						continue
					}

					time.Sleep(10 * time.Second)
				}
			}
		}(target)
	}

	workerWg.Wait()
	logging.LogInfo("ConnectToRelays: All connection attempts stopped", nil)
}

// sendLinkPacket sends a LINK packet to the specified address.
func (r *Relay) sendLinkPacket(targetAddr string) error {
	encodedCallsign, err := EncodeCallsign(r.RelayCallsign)
	if err != nil {
		return err
	}

	packet := make([]byte, 10)
	copy(packet[:4], []byte(MagicLink))
	copy(packet[4:], encodedCallsign)

	udpAddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return err
	}

	_, err = r.Socket.WriteToUDP(packet, udpAddr)
	return err
}

// LogPeerState logs the state of connected clients and relays every 10 seconds.
func (r *Relay) LogPeerState(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			logging.LogInfo("LogPeerState: Shutdown signal received, stopping...", nil)
			return
		case <-tick.C:
			numClients := 0
			numRelays := 0
			r.Clients.Range(func(key, value interface{}) bool {
				numClients++
				return true
			})
			r.LinkedRelays.Range(func(key, value interface{}) bool {
				numRelays++
				return true
			})
			logging.LogDebug("Current peers:", map[string]interface{}{
				"clients": numClients,
				"relays":  numRelays,
			})
			r.Clients.Range(func(key, value interface{}) bool {
				client := value.(*ClientState)
				logging.LogDebug("Client",
					map[string]interface{}{
						"key":      key,
						"callsign": client.Callsign,
						"lastPong": client.LastPong.Format(time.RFC3339),
					},
				)
				return true
			})
			r.LinkedRelays.Range(func(key, value interface{}) bool {
				relay := value.(*LinkState)
				logging.LogDebug("Relay",
					map[string]interface{}{
						"key":      key,
						"callsign": relay.Callsign,
						"lastPong": relay.LastPong.Format(time.RFC3339),
					},
				)
				return true
			})
		}
	}
}

// relayDataPacket relays a data packet to all connected clients and relays.
func (r *Relay) relayDataPacket(packet []byte, senderAddr *net.UDPAddr) {
	logging.LogDebug("Relaying data packet to other clients", map[string]interface{}{"from": senderAddr})

	// Check if the sender is a listen-only client
	if client, exists := r.Clients.Load(senderAddr.String()); exists {
		if client.(*ClientState).ListenOnly {
			logging.LogDebug("Ignoring data packet from listen-only client", map[string]interface{}{"from": senderAddr})
			return
		}
	}

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

	r.LinkedRelays.Range(func(key, value interface{}) bool {
		addr := key.(string)
		if addr != senderAddr.String() {
			logging.LogDebug("Forwarding data packet to relay", map[string]interface{}{"to": addr})
			udpAddr, err := net.ResolveUDPAddr("udp", addr)
			if err != nil {
				logging.LogError("Error resolving relay address", map[string]interface{}{"addr": addr, "err": err})
				return true
			}

			_, err = r.Socket.WriteToUDP(packet, udpAddr)
			if err != nil {
				logging.LogError("Error forwarding data packet to relay", map[string]interface{}{"addr": addr, "err": err})
			}
		}
		return true
	})
	// Update the last data packet time for the sender
	if client, exists := r.Clients.Load(senderAddr.String()); exists {
		client.(*ClientState).LastDataPacket = time.Now()
	} else if relay, exists := r.LinkedRelays.Load(senderAddr.String()); exists {
		relay.(*LinkState).LastDataPacket = time.Now()
	}
	r.UpdateMetrics() // Update metrics when a data packet is received
}

func (r *Relay) Close() error {
	if r.Socket != nil {
		return r.Socket.Close()
	}
	return nil
}
