# Simple DNS Proxy

A lightweight DNS proxy server written in Go that handles A records based on configuration and relays other queries to a fallback DNS server.

## Features

- Serves DNS A records based on entries in a YAML configuration file
- Supports both UDP and TCP DNS protocols
- Relays queries not found in configuration to a fallback DNS server
- Automatically reloads configuration when the config file changes
- Configurable network interface binding and port settings
- Cross-platform support (Linux, Windows)

## Configuration

The DNS proxy is configured via a YAML file (`config.yaml`). Here's a sample configuration:

```yaml
records:
  hws.unreal.local:
    ip: 172.31.255.80
    ttl: 3600 # Optional TTL for this specific record
  euc.unreal.local:
    ip: 172.31.255.80
  unreal.local:
    ip: 172.31.255.135
fallback_dns: 8.8.8.8
fallback_protocol: "udp" # or "tcp", blank means use incoming protocol
default_ttl: 60 # Default TTL for records without a specific TTL
server:
  udp:
    enabled: true
    port: 53
    interfaces: ["0.0.0.0"] # List of IPs to bind to. "0.0.0.0" means all interfaces.
  tcp:
    enabled: true
    port: 53
    interfaces: ["0.0.0.0", "127.0.0.1"] # Example of binding to multiple IPs
```

### Configuration Options

- `records`: A map of domain names to record configurations.
  - `ip`: The IP address for the A record.
  - `ttl`: (Optional) The TTL for this specific record in seconds.
- `fallback_dns`: The DNS server to relay queries to when not found in `records`
- `fallback_protocol`: The protocol to use for relaying queries ("udp", "tcp"). If blank, it uses the incoming protocol.
- `default_ttl`: (Optional) The default TTL for records that don't have a specific TTL. Defaults to 3600.
- `server`: Server configuration section
  - `udp`: UDP server settings
    - `enabled`: Whether to enable the UDP server (boolean)
    - `port`: Port number for the UDP server
    - `interfaces`: List of network interfaces to bind to (e.g., ["0.0.0.0", "192.168.1.100"])
  - `tcp`: TCP server settings
    - `enabled`: Whether to enable the TCP server (boolean) 
    - `port`: Port number for the TCP server
    - `interfaces`: List of network interfaces to bind to (e.g., ["0.0.0.0", "127.0.0.1"])

## Building and Running

### Using Docker

1. Build and run the Docker container:
   ```
   ./build_and_run.sh
   ```

2. The DNS proxy will be available on port 53 (UDP and TCP).

### Cross-compiling for Different Platforms

#### Option 1: With Go installed locally

If you have Go installed on your machine, you can use the local cross-compilation script:

```
./build_cross_compile.sh
```

#### Option 2: Using Docker (no Go installation required)

If you don't have Go installed but have Docker available, you can use the Docker-based cross-compilation script:

```
./docker_build.sh
```

Both cross-compilation methods will create binaries for:
- Linux (Debian) x86_64
- Windows x86_64

The compiled binaries will be available in the `build` directory as:
- `simple-dns-proxy-linux-amd64.tar.gz`
- `simple-dns-proxy-windows-amd64.zip`

## Usage

### Running the standalone binary

1. Download or build the appropriate binary for your platform
2. Create or edit the `config.yaml` file
3. Run the binary:
   ```
   ./simple-dns-proxy    # Linux
   simple-dns-proxy.exe  # Windows
   ```

### Testing

You can test the DNS server using tools like `dig` or `nslookup`:

```
dig @localhost hws.unreal.local
nslookup hws.unreal.local localhost
```

## License

MIT
