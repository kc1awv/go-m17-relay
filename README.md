# go-m17-relay

## Overview
This project is an M17 Relay server that handles client connections, relay interlinks, and communication between them over UDP. It supports various packet types for managing connections, relays, and client states.

If you're familiar with how traditional M17 (and other) relfectors work, this is the same idea but keeps it simple with only one 'module' to link to. In fact, an M17 Relay has no concept of modules at all. With that in mind, M17 Relays will accept connections from M17 Clients that still use CONN packets with modules, but the Relay will silently ignore them. All clients connecting to a Relay will be in the same 'room' as it were.

Multiple Relays can be interlinked together. The interlinking concept should be handled as a hub-and-spoke topology. While a mesh topology _could_ be created, this is untested at this time.

## Glossary

### Client
An end "user". Some client programs include:
  - [mvoice](https://github.com/n7tae/mvoice)
  - [M17Gateway](https://github.com/g4klx/M17Gateway)
  - [M17Client](https://github.com/g4klx/M17Client)
  - [DroidStar](https://github.com/nostar/DroidStar)

### Relay
This program. Relays connect M17 Clients or other M17 Relays to each other and passes data between them.

### Peer
Either a Client or a Relay. Peer is a general term used for anything connecting to a Relay.

### Control Packet
Packets sent or received by Peers to or from a Relay for creating, maintaining, and tearing down connections to each other.

### Data Packet
Packets sent by Clients to a Relay for distribution to other Peers. Data packets may contain M17 voice stream or packet data.

## Project Structure
```
main.go         - Program Main functions

config/
    config.go   - Configuration functions

metrics/
    metrics.go  - Metrics functions

logging/
    logging.go  - Log processing functions

relay/
    encoding.go - M17 base-40 address encoding
    relay.go    - Relay functions

config.dist     - Example configuration file
go.mod          - Go modules
go.sum          - Go module checksums
LICENSE         - GPL v3 License file
README.md       - This README file
```

## Configuration
The configuration is loaded from a JSON file (`config.json`).

The configuration file should include the following fields:

- `log_level`: The logging level (e.g., "debug", "info", "warn", "error").
- `relay_callsign`: The 'callsign' of the relay.
- `bind_address`: The address to bind the UDP socket to.
- `target_relays`: A list of target relay addresses to connect to.

Example config.json:

```json
{
    "log_level": "debug",
    "relay_callsign": "RLY000001",
    "bind_address": "127.0.0.1:17000",
    "web_interface_address": "127.0.0.1:8080",
    "target_relays": [
        {
            "callsign": "RLY000002",
            "address": "127.0.0.1:17001"
        }
    ]
}
```

## Packet Types

### Control Packets

### `CONN`
#### Connection request packet. Sent by a Client for linking to a Relay.

| Byte | Size    | Purpose                                                                                                        |
-------|---------|-------------------------------------------------------------------------------------------------------------------
| 0-3  | 4 bytes | Magic - ASCII "CONN"                                                                                             |
| 4-9  | 6 bytes | 'from' callsign as encoded per [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A)   |
| 10   | 1 byte  | ASCII (A-Z) module to connect to (Optional. Ignored for relays, kept for legacy mrefd/urfd reflectors)           |

### `LINK`
#### Link request packet. Sent by a Relay to interlink with another Relay.

| Byte | Size    | Purpose                                                                                                          |
-------|---------|-------------------------------------------------------------------------------------------------------------------
| 0-3  | 4 bytes | Magic - ASCII "LINK"                                                                                             |
| 4-9  | 6 bytes | 'from' callsign as encoded per [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A)   |

### `ACKN`
#### Acknowledgment packet. Used to acknowledge a successful `CONN` or `LINK`.

| Byte | Size    | Purpose              |
-------|---------|-----------------------
| 0-3  | 4 bytes | Magic - ASCII "ACKN" |

### `NACK`
#### Negative acknowledgment packet. Used to indicate an error with a `CONN` or `LINK`.

| Byte | Size    | Purpose              |
-------|---------|-----------------------
| 0-3  | 4 bytes | Magic - ASCII "NACK" |

### `PING`
#### Ping packet. Sent to maintain Peer keepalive.

| Byte | Size    | Purpose                                                                                                          |
-------|---------|-------------------------------------------------------------------------------------------------------------------
| 0-3  | 4 bytes | Magic - ASCII "PING"                                                                                             |
| 4-9  | 6 bytes | 'from' callsign as encoded per [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A)   |

### `PONG`
#### Pong packet. Reply to a `PING` to acknowledge keepalive.

| Byte | Size    | Purpose                                                                                                          |
-------|---------|-------------------------------------------------------------------------------------------------------------------
| 0-3  | 4 bytes | Magic - ASCII "PONG"                                                                                             |
| 4-9  | 6 bytes | 'from' callsign as encoded per [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A)   |

### `DISC`
#### Disconnect packet. Used to politely close a connection to/from a Peer.

| Byte | Size    | Purpose                                                                                                          |
-------|---------|-------------------------------------------------------------------------------------------------------------------
| 0-3  | 4 bytes | Magic - ASCII "DISC"                                                                                             |
| 4-9  | 6 bytes | 'from' callsign as encoded per [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A)   |

---

### Data Packets

### `M17 `
#### M17 stream packet. Used to transfer voice stream data.

| Field          | Size     | Description                                                                                                                                   |
|----------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| MAGIC          | 4 bytes  | Magic bytes 0x4d313720 `(M17 )`                                                                                                               |
| StreamID (SID) | 2 bytes  | Random bits, changed for each PTT or stream, but consistent from frame to frame within a stream                                               |
| LICH           | 28 bytes | The meaningful contents of a LICH frame defined in the [M17 Protocol Specification](https://spec.m17project.org/pdf/M17_spec.pdf#section.2.5) |
| FN             | 2 bytes  | Frame number including the last frame indicator at (FN & 0x8000)                                                                              |
| Payload        | 16 bytes | Payload (exactly as would be transmitted in an RF stream frame)                                                                               |
| Reserved       | 2 bytes  | Reserved two byte field for future use. Originally CRC16, but is not needed over IP                                                           |

Total: 54 bytes (432 bits)

---

## Functions

### Main Functions

- `main()`: The entry point of the application. It loads the configuration, initializes the relay, and starts the services.
- `validateConfig(cfg *config.Config) error`: Validates the configuration.
- `isValidIPAddress(addr string)`: Validates bind and target relay addresses.
- `startServices(ctx context.Context, r *relay.Relay, wg *sync.WaitGroup)`: Starts the relay services.

### Relay Functions

- `NewRelay(addr string, callsign string) *Relay`: Initializes a new Relay object.
- `Listen(ctx context.Context, wg *sync.WaitGroup)`: Starts receiving packets and processes them based on the packet type.
- `ConnectToRelays(ctx context.Context, wg *sync.WaitGroup)`: Connects to target relays.
- `PingPeers(ctx context.Context, wg *sync.WaitGroup)`: Pings connected peers.
- `RemoveInactivePeers(ctx context.Context, wg *sync.WaitGroup)`: Removes inactive peers.
- `LogPeerState(ctx context.Context, wg *sync.WaitGroup)`: Logs the state of connected peers.
- `sendLinkPacket(targetAddr string) error`: Sends a LINK packet to the specified target address.
- `sendPing(addr string)`: Sends a PING packet to the specified client.
- `sendPong(addr *net.UDPAddr, callsign string)`: Sends a PONG packet to the specified address.
- `sendPacket(magic string, addr *net.UDPAddr, callsign []byte)`: Sends a control packet to the specified address.
- `handleControlPacket(data []byte, addr *net.UDPAddr)`: Processes incoming control packets.
- `handleConnPacket(callsign string, addr *net.UDPAddr, module byte)`: Processes a connection request (CONN packet).
- `handleLinkPacket(callsign string, addr *net.UDPAddr)`: Processes a link request (LINK packet).
- `handleAcknPacket(addr *net.UDPAddr)`: Processes an acknowledgment (ACKN) packet.
- `handlePingPacket(callsign string, addr *net.UDPAddr)`: Processes a PING packet and responds with a PONG packet.
- `handlePongPacket(callsign string, addr *net.UDPAddr)`: Processes a PONG packet.
- `handleDiscPacket(callsign string, addr *net.UDPAddr)`: Processes a DISCONNECT (DISC) packet and removes the client.
- `relayDataPacket(packet []byte, senderAddr *net.UDPAddr)`: Handles data packets from clients.

### Metrics Functions
- `NewMetricsCollector(relayCallsign string) *MetricsCollector`: Creates a new MetricsCollector.
- `UpdateMetrics(clients, relays map[string]PeerInfo)`: Updates the metrics.
- `GetMetrics() Metrics`: Returns the current metrics.
- `GetPeerInfo() (clients, relays []PeerInfo)`: Returns the current peer information.
- `ServeMetrics(w http.ResponseWriter, req *http.Request)`: Serves the metrics as a JSON response.
- `ServeWebInterface(w http.ResponseWriter, req *http.Request)`: Serves a simple web interface to display the metrics.

### Logging Functions

- `InitLogLevel(level string)`: Initializes the logging level.
- `LogDebug(message string, fields map[string]interface{})`: Logs a debug message.
- `LogInfo(message string, fields map[string]interface{})`: Logs an info message.
- `LogWarn(message string, fields map[string]interface{})`: Logs a warning message.
- `LogError(message string, fields map[string]interface{})`: Logs an error message.

## Usage

1. Configure the relay server by copying `config.dist` to `config.json` and editing the `config.json` file.
2. Run the relay server using the following command:
   ```sh
   go run main.go
   ```
3. The relay server will start and log messages based on the configured log level.

## License
This project is licensed under the GNU General Public License v3.0. See the LICENSE file for more details.

## Contributing
Contributions are welcome! Please open an issue or submit a pull request on GitHub.
