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
  hws.unreal.local: 172.31.255.80
  euc.unreal.local: 172.31.255.80
  hws.euc.unreal.local: 172.31.255.80
  unreal.local: 172.31.255.135
fallback_dns: 8.8.8.8
server:
  udp:
    enabled: true
    port: 53
    interface: "" # Empty string means all interfaces
  tcp:
    enabled: true
    port: 53
    interface: "" # Empty string means all interfaces
```

### Configuration Options

- `records`: A map of domain names to IP addresses for A records
- `fallback_dns`: The DNS server to relay queries to when not found in `records`
- `server`: Server configuration section
  - `udp`: UDP server settings
    - `enabled`: Whether to enable the UDP server (boolean)
    - `port`: Port number for the UDP server
    - `interface`: Network interface to bind to (empty for all)
  - `tcp`: TCP server settings
    - `enabled`: Whether to enable the TCP server (boolean) 
    - `port`: Port number for the TCP server
    - `interface`: Network interface to bind to (empty for all)

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
