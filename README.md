# go-m17-relay

## Overview
This project is an M17 Relay server that handles client connections, relay interlinks, and communication between them over UDP. It supports various packet types for managing connections, relays, and client states.

If you're familiar with how traditional M17 (and other) reflectors work, this is the same idea but keeps it simple with only one 'module' to link to. In fact, an M17 Relay has no concept of modules at all. With that in mind, M17 Relays will accept connections from M17 Clients that still use CONN packets with modules, but the Relay will silently ignore them. All clients connecting to a Relay will be in the same 'room' as it were.

Multiple Relays can be interlinked together. The interlinking concept should be handled as a hub-and-spoke topology. While a mesh topology _could_ be created, it is untested at this time.

## Demo Relay
A demo relay is hosted at [KC1AWV.net](https://relay.kc1awv.net)

- Relay Callsign: RLYKC1AWV
- Hostname: relay.kc1awv.net
- UDP Client Port: 17000

An opt-in call-home service is available for listing relays that are made publicly available, and can be found at the [M17 Public Relay Status](https://relay.m17.link) page. There is an API endpoint that can also be used to retrieve a list of available relays in JSON format at [M17 Public Relay List API](https://relay.m17.link/api/v1/relays).

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
Packets sent by Clients to a Relay for distribution to other Peers. Data packets may contain M17 voice stream or (in the future) packet data.

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

callhome/
    callhome.go - Call-home functions

config.dist     - Example configuration file
go.mod          - Go modules
go.sum          - Go module checksums
LICENSE         - GPL v3 License file
README.md       - This README file
```

## Configuration
The configuration is loaded from a JSON file (`config.json`).

The configuration file should include the following fields:

- `log_level`: (`STRING`) The logging level (e.g., "debug", "info", "warn", "error").
- `relay_callsign`: (`STRING`) The 'callsign' of the relay. 9 character maximum, only characters allowed by [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A).
- `bind_address`: (`STRING`) The address and port to bind the UDP socket to.
- `web_interface_address`: (`STRING`) The address and port to bind the web interface to.
- `public_ip`: (`STRING`) The public IP address of the Relay, used only for call-home.
- `daemon_mode`: (`BOOL`) Daemonize the relay to run in the background.
- `pid_file`: (`STRING`) The location where the PID file should be created (daemon mode)
- `log_file`: (`STRING`) The location where the log file should be created (daemon mode)
- `uuid`: (`STRING`) The unique identifier for the relay. This will be generated and updated by the call-home service if not provided.
- `call_home_enabled`: (`BOOL`) Enable or disable the call-home service.
- `target_relays`: (`ARRAY`) A list of target relay addresses to connect to.
  - `callsign`: (`STRING`) 'Callsign' of the relay to connect to.
  - `address`: (`STRING`) Address and port of the relay to connect to.

Example config.json:

```json
{
    "log_level": "debug",
    "relay_callsign": "RLY000001",
    "bind_address": "127.0.0.1:17000",
    "web_interface_address": "127.0.0.1:8080",
    "public_ip": "",
    "daemon_mode": true,
    "pid_file": "/var/run/go-m17-relay/relay.pid",
    "log_file": "/var/log/go-m17-relay/relay.log",
    "uuid": "",
    "call_home_enabled": true,
    "target_relays": [
        {
            "callsign": "RLY000002",
            "address": "127.0.0.1:17001"
        }
    ]
}
```

## Call-Home Service

The call-home service allows the relay to periodically update its status and configuration with a central server. This service can be enabled or disabled via the `call_home_enabled` configuration option.

### How It Works

 0. **Information Gathering**: The relay will parse this information from the configuration file:
   
   - Relay Callsign
   - Relay Public IP Address (if provided)
   - Relay Listening Port
   - Relay UUID (if provided)

   If a public IP address is not provided in the configuration file, the relay will call out to a third-party service (`icanhazip.com` for now, this may change later) to determine your public IP address. This is beneficial to sysops that are using dynamic public addresses that may change.

 1. **Initial Call-Home**: The relay will send an initial call-home request on startup.
 2. **Periodic Updates**: The relay will send periodic updates every 7.5 minutes with a random delay within each interval.

### Configuration Fields

- `public_ip`: The public IP address of the Relay. If you have a static public IP, set it here. If you have a dynamic public IP, leave it blank, and the public IP will be discovered using icanhazip.com.
- `uuid`: The unique identifier for the Relay. If not provided, it will be generated by the call-home service, and saved to the configuration file.
- `call_home_enabled`: Enable or disable the call-home service.

### Example Call-Home Payload

```json
{
    "uuid": "87886fd8-0b28-47af-9580-cf0a1350a5ea",
    "callsign": "KC1AWV",
    "ip_address": "192.168.1.120",
    "udp_port": 17000,
    "status": true
}
```
### Headers

The call-home request includes the following headers:

- `Content-Type`: `application/json`
- `X-Relay-Program`: `go-m17-relay_v0.0.1`

## Packet Types

### Control Packets

### `CONN`
#### Connection request packet. Sent by a Client for linking to a Relay.

| Byte | Size    | Purpose                                                                                                                  |
-------|---------|---------------------------------------------------------------------------------------------------------------------------
| 0-3  | 4 bytes | Magic - ASCII "CONN"                                                                                                     |
| 4-9  | 6 bytes | 'from' callsign as encoded per [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A)           |
| 10   | 1 byte  | ASCII (A-Z) module to connect to (Optional. Ignored for relays, kept for backwards compatibilty with mrefd/urfd clients) |

### `LINK`
#### Link request packet. Sent by a Relay to interlink with another Relay.

| Byte | Size    | Purpose                                                                                                          |
-------|---------|-------------------------------------------------------------------------------------------------------------------
| 0-3  | 4 bytes | Magic - ASCII "LINK"                                                                                             |
| 4-9  | 6 bytes | 'from' callsign as encoded per [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A)   |

### `LSTN`
#### Listen request packet. Sent by a listen-only client to link with a Relay.

| Byte | Size    | Purpose                                                                                                          |
-------|---------|-------------------------------------------------------------------------------------------------------------------
| 0-3  | 4 bytes | Magic - ASCII "LSTN"                                                                                             |
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

### `INFO?`
#### Relay information query

| Byte | Size    | Purpose              |
-------|---------|-----------------------
| 0-3  | 4 bytes | Magic - ASCII "INFO" |
| 4    | 1 byte  | ASCII character '?'  |

### `INFO`
#### Relay information reply

| Byte  | Size    | Purpose                                                                                                        |
--------|---------|----------------------------------------------------------------------------------------------------------------|
| 0-3   | 4 bytes | Magic - ASCII "INFO"                                                                                           |
| 4-9   | 6 bytes | 'from' callsign as encoded per [M17 Address Encoding](https://spec.m17project.org/pdf/M17_spec.pdf#appendix.A) |
| 10-13 | 4 bytes | Relay uptime in seconds                                                                                        |
| 14-15 | 2 bytes | Number of Clients connected to Relay                                                                           |
| 16-17 | 2 bytes | Number of Relays interlinked to Relay                                                                          |

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
| LSD            | 28 bytes | The Link Setup Data (DST, SRC, TYPE, META field) as defined in section 2.5.1 of the [M17 Protocol Specification](https://spec.m17project.org/) |
| FN             | 2 bytes  | Frame number including the last frame indicator at (FN & 0x8000)                                                                              |
| Payload        | 16 bytes | Payload (exactly as would be transmitted in an RF stream frame)                                                                               |
| CRC16          | 2 bytes  | CRC for the entire packet, as defined in Section 2.6 of the [M17 Protocol Specification](https://spec.m17project.org/) |

Total: 54 bytes (432 bits)

#### M17 two-packets header packet. Used to transfer voice stream data in Two Packets mode.

| Field          | Size     | Description                                                                                                                                   |
|----------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| MAGIC          | 4 bytes  | Magic bytes 0x4d313748 `(M17H)`                                                                                                               |
| StreamID (SID) | 2 bytes  | Random bits, changed for each PTT or stream, but consistent from frame to frame within a stream                                               |
| LSD            | 28 bytes | The Link Setup Data (DST, SRC, TYPE, META field) as defined in section 2.5.1 of the [M17 Protocol Specification](https://spec.m17project.org/) |
| CRC16          | 2 bytes  | CRC for the entire packet, as defined in Section 2.6 of the [M17 Protocol Specification](https://spec.m17project.org/)                        |

Total: 36 bytes (288 bits)

#### M17 two-packets data packet. Used to transfer voice stream data in Two Packets mode.

| Field          | Size     | Description                                                                                                                                   |
|----------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| MAGIC          | 4 bytes  | Magic bytes 0x4d313744 `(M17D)`                                                                                                               |
| StreamID (SID) | 2 bytes  | Random bits, changed for each PTT or stream, but consistent from frame to frame within a stream                                               |
| FN             | 2 bytes  | Frame number exactly as would be transmitted as an RF stream frame, including the last frame indicator at (FN & 0x8000)                       |
| Payload        | 16 bytes | Payload (exactly as would be transmitted in an RF stream frame)                                                                               |
| CRC16          | 2 bytes  | CRC for the entire packet, as defined in Section 2.6 of the [M17 Protocol Specification](https://spec.m17project.org/)                        |

Total: 26 bytes (208 bits)

#### M17 Packet Mode data packet. Used to transfer packet data.

| Field          | Size         | Description                                                                                                                                   |
|----------------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| MAGIC          | 4 bytes      | Magic bytes 0x4d313750 `(M17P)`                                                                                                               |
| LSF            | 30 bytes     | The Link Setup Frame(DST, SRC, TYPE, META field, CRC) as defined in Table 2.5.2 of the [M17 Protocol Specification](https://spec.m17project.org/) |
| Payload        | 4..825 bytes | This includes a type specifer, the user data, and a CRC                                                                                       |

Total: 38..859 bytes (304..6872 bits)

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
- `handleLstnPacket(callsign string, addr *net.UDPAddr)`: Processes a listen request (LSTN packet).
- `handleAcknPacket(addr *net.UDPAddr)`: Processes an acknowledgment (ACKN) packet.
- `handlePingPacket(callsign string, addr *net.UDPAddr)`: Processes a PING packet and responds with a PONG packet.
- `handlePongPacket(callsign string, addr *net.UDPAddr)`: Processes a PONG packet.
- `handleInfoPacket(addr *net.UDPAddr)`: Processes an INFO? (query) packet and responds with an INFO packet.
- `handleDiscPacket(callsign string, addr *net.UDPAddr)`: Processes a DISCONNECT (DISC) packet and removes the client.
- `relayDataPacket(packet []byte, senderAddr *net.UDPAddr)`: Handles data packets from clients.

### Metrics Functions
- `NewMetricsCollector(relayCallsign string) *MetricsCollector`: Creates a new MetricsCollector.
- `UpdateMetrics(clients, relays map[string]PeerInfo)`: Updates the metrics.
- `GetMetrics() Metrics`: Returns the current metrics.
- `InitializeHTTPServer(ctx context.Context, wg *sync.WaitGroup, cfg *config.Config)`: Initializes and starts the HTTP server.
- `ServeMetrics(w http.ResponseWriter, req *http.Request)`: Serves the metrics as a JSON response.
- `ServeWebInterface(w http.ResponseWriter, req *http.Request)`: Serves a simple web interface to display the metrics.

### Logging Functions

- `InitLogLevel(level string)`: Initializes the logging level.
- `LogDebug(message string, fields map[string]interface{})`: Logs a debug message.
- `LogInfo(message string, fields map[string]interface{})`: Logs an info message.
- `LogWarn(message string, fields map[string]interface{})`: Logs a warning message.
- `LogError(message string, fields map[string]interface{})`: Logs an error message.

### CallHome Functions

- `CallHome(ctx context.Context, cfg *config.Config)`: Sends a call-home request to the central server.
- `StartPeriodicCallHome(ctx context.Context, wg *sync.WaitGroup, cfg *config.Config)`: Starts the periodic call-home service.
- `getExternalIP() (string, error)`: Retrieves the external IP address of the relay.
- `saveConfig(cfg *config.Config) error`: Saves the updated configuration to the config file.

## Usage

1. Configure the relay server by copying `config.dist` to `config.json` and editing the `config.json` file.
2. Run the relay server using the following command:

> [!TIP]
> Config file location optional, defaults to working directory

   ```sh
   go run main.go [-config /path/to/config.json]
   ```
3. The relay server will start and log messages based on the configured log level.

## License
This project is licensed under the GNU General Public License v3.0. See the LICENSE file for more details.

## Contributing
Contributions are welcome! Please open an issue or submit a pull request on GitHub.
