# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PowerDNS Load Balancer (ploadb) is a DNS-based load balancer service that monitors multiple IP addresses for DNS A records and automatically enables/disables them based on configurable health checks (ping, HTTP, or HTTPS). It integrates with PowerDNS via its HTTP API to provide automatic failover and load distribution at the DNS level.

## Core Architecture

The service operates as a single Go binary (`ploadb/ploadb.go`) that runs as a Linux systemd service. It follows this flow:
- **Main Loop**: Every 20 seconds, queries PowerDNS API for all zones
- **Zone Processing**: For each zone, identifies A records with multiple IP addresses
- **Health Checking**: Performs configurable health checks (ping, HTTP, or HTTPS) based on record configuration
- **DNS Updates**: Updates PowerDNS records to disable unreachable hosts via PATCH API calls

Key functions:
- `DoWork()` - Main processing loop (20-second intervals)
- `process_domain(domain)` - Processes individual zones
- `handle_load_balance()` - Performs health checks and updates records
- `parseProbeConfig()` - Parses JSON probe configuration from record comments
- `performProbe()` - Executes appropriate probe type (ping/HTTP/HTTPS)
- `getdomainlist()`, `getdomain()`, `send_update()` - PowerDNS API interaction

## Build and Development Commands

### Building the Project
```bash
cd ploadb
go build -o ploadb ploadb.go
```

### Testing
The project includes test scripts for API interaction:
- `test.sh` - Tests DNS updates and resolution (uses curl and nslookup)
- `getzones.sh` - Lists all PowerDNS zones
- `getit.sh` - Retrieves specific zone information

No formal unit tests or linting tools are configured in the codebase.

### Installation
```bash
# Copy binary to system location
sudo cp ploadb /usr/local/bin/
sudo chmod +x /usr/local/bin/ploadb
sudo setcap cap_net_raw=+ep /usr/local/bin/ploadb  # Required for ICMP ping

# Install systemd service
sudo cp etc/systemd/system/ploadb.service /etc/systemd/system/
sudo systemctl daemon-reload
```

## Configuration

### Service Configuration
Configuration is managed via TOML file at `/etc/ploadb.conf`:
```toml
Baseurl = "http://localhost:8081"        # PowerDNS API URL
ApiPassword = "your-api-key-here"        # PowerDNS API key
```

### Health Check Configuration
Health check behavior is configured per DNS record using PowerDNS record comments in JSON format:

#### Ping Probe (Default)
```json
{"type": "ping", "timeout": 5}
```
- Sends 3 ICMP packets
- Considers healthy if any packet is received
- Default behavior if no comment is provided

#### HTTP Probe
```json
{"type": "http", "path": "/health", "port": 8080, "timeout": 10, "expected": 200}
```
- Makes GET request to specified path
- Considers healthy if HTTP status matches expected code

#### HTTPS Probe
```json
{"type": "https", "path": "/api/status", "port": 443, "timeout": 5, "expected": 200}
```
- Makes HTTPS GET request (certificate verification disabled)
- Considers healthy if HTTP status matches expected code

#### Configuration Parameters
- `type`: "ping", "http", or "https" (default: "ping")
- `path`: HTTP(S) path to check (default: "/")
- `port`: Port number (defaults: 80 for http, 443 for https)
- `timeout`: Timeout in seconds (default: 5)
- `expected`: Expected HTTP status code (default: 200)

The service only monitors A records with multiple IP addresses. Single-IP A records and other record types (AAAA, CNAME, etc.) are ignored.

## Dependencies

Key Go modules (see `go.mod`):
- `github.com/go-resty/resty` - HTTP client for PowerDNS API
- `github.com/oilbeater/go-ping` - ICMP ping implementation
- `github.com/tidwall/gjson`, `github.com/tidwall/sjson` - JSON processing
- `github.com/kardianos/service` - Cross-platform service management
- `github.com/BurntSushi/toml` - Configuration file parsing
- `gopkg.in/natefinch/lumberjack.v2` - Log rotation

## File Structure

```
pdnsloadbalancer/
├── ploadb/
│   ├── ploadb.go           # Main application (single file)
│   ├── etc/ploadb.conf     # Configuration template
│   └── ploadb              # Compiled binary
├── etc/systemd/system/
│   └── ploadb.service      # Systemd service file
├── build.sh                # Simple build script
├── test.sh                 # API testing script
├── *.json                  # Example PowerDNS API payloads
└── documentation files     # Comprehensive docs in *.md files
```

## Key Implementation Details

- **Concurrency**: Uses goroutines for concurrent zone processing and health check operations
- **Error Handling**: Basic error handling with log output; service continues on API/network errors
- **State Management**: Uses PowerDNS record `disabled` field (false=healthy, true=unhealthy)
- **Logging**: Writes to `/var/log/ploadb/ploadb.log` with automatic rotation (5MB max, 3 backups, 28-day retention)
- **Timing**: Hard-coded 20-second intervals between health check cycles
- **Probe Configuration**: JSON configuration stored in PowerDNS record comments
- **HTTP Probes**: Support custom paths, ports, timeouts, and expected status codes
- **HTTPS Probes**: Certificate verification disabled for simplicity
- **Backward Compatibility**: Existing configurations default to ping probes
- **Privileges**: Requires root or `cap_net_raw` capability for ICMP ping
- **Failsafe Mode**: When all hosts in a load-balanced record fail health checks, the first entry is automatically enabled to ensure DNS queries always return at least one IP address

## PowerDNS Integration

The service communicates with PowerDNS HTTP API using these endpoints:
- `GET /api/v1/servers/localhost/zones` - List zones
- `GET /api/v1/servers/localhost/zones/{zone}` - Get zone details
- `PATCH /api/v1/servers/localhost/zones/{zone}` - Update records

Authentication via `X-API-Key` header matching PowerDNS `api-key` configuration.

## Web GUI Dashboard

PowerDNS Load Balancer includes a built-in web dashboard for monitoring health check status in real-time. The GUI provides:

- **Real-time Status**: Live updates of all monitored zones and their health states
- **Visual Indicators**: Color-coded status badges (ENABLED/DISABLED) for each IP address
- **Detailed Information**: Shows last check times, probe types, and configuration details
- **Sorted Display**: IP addresses are displayed in numerical order for easy reading
- **Auto-refresh**: WebSocket connection provides instant updates without page refresh

### Accessing the GUI

The web interface is available at `http://localhost:8080` by default. The port can be configured in `/etc/ploadb.conf`:

```toml
WebPort = "8080"  # Default web GUI port
```

### GUI Features

![PowerDNS Load Balancer GUI](ploadb-gui-screenshot.png)

The dashboard displays:
- **Zone Organization**: Zones are grouped and clearly labeled with folder icons
- **Hostname Grouping**: Each hostname shows all associated IP addresses
- **Status Badges**: Green "ENABLED" and red "DISABLED" badges for quick status identification
- **Timestamps**: Last health check time for each IP address
- **Probe Types**: Displays the type of health check being performed (ping, http, https, tcp)
- **Live Updates**: Real-time status changes via WebSocket connection

### Service Management via GUI

While the GUI is read-only for monitoring, it provides comprehensive visibility into:
- Current health status of all monitored endpoints
- Historical state changes and timing information
- Active probe configurations for each record
- Overall system health at a glance

For configuration changes, continue to use the PowerDNS API directly as shown in the examples below.

## Example Usage

### Setting up HTTP Health Checks

1. **Create A record with multiple IPs and HTTP probe configuration:**
```bash
curl -X PATCH http://localhost:8081/api/v1/servers/localhost/zones/example.com \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [{
      "name": "api.example.com",
      "type": "A",
      "changetype": "REPLACE",
      "records": [
        {"content": "10.0.1.10", "disabled": false},
        {"content": "10.0.1.11", "disabled": false}
      ],
      "comment": "{\"type\":\"http\",\"path\":\"/health\",\"port\":8080,\"timeout\":5,\"expected\":200}"
    }]
  }'
```

2. **Monitor the logs to see health check results:**
```bash
sudo journalctl -u ploadb -f
```

### Setting up HTTPS Health Checks

```bash
curl -X PATCH http://localhost:8081/api/v1/servers/localhost/zones/example.com \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [{
      "name": "secure-api.example.com",
      "type": "A",
      "changetype": "REPLACE",
      "records": [
        {"content": "10.0.1.20", "disabled": false},
        {"content": "10.0.1.21", "disabled": false}
      ],
      "comment": "{\"type\":\"https\",\"path\":\"/api/status\",\"port\":443,\"timeout\":10,\"expected\":200}"
    }]
  }'
```

### Mixed Configuration Example

You can have different probe types for different records in the same zone:

```bash
# Web servers with HTTP health checks
curl -X PATCH http://localhost:8081/api/v1/servers/localhost/zones/example.com \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [{
      "name": "web.example.com",
      "type": "A",
      "changetype": "REPLACE",
      "records": [
        {"content": "10.0.1.30", "disabled": false},
        {"content": "10.0.1.31", "disabled": false}
      ],
      "comment": "{\"type\":\"http\",\"path\":\"/\",\"port\":80}"
    }]
  }'

# Database servers with ping health checks (default)
curl -X PATCH http://localhost:8081/api/v1/servers/localhost/zones/example.com \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [{
      "name": "db.example.com",
      "type": "A",
      "changetype": "REPLACE",
      "records": [
        {"content": "10.0.1.40", "disabled": false},
        {"content": "10.0.1.41", "disabled": false}
      ]
    }]
  }'
```

### Real-World Production Examples

These examples show actual configurations from production gw.lo and apps.gw.lo zones:

#### Kubernetes API Load Balancing with TCP Health Checks

```bash
# Kubernetes control plane nodes with TCP port 6443 health checks
curl -X PATCH http://192.168.1.51:8081/api/v1/servers/localhost/zones/gw.lo. \
  -H "X-API-Key: quest.5124" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [{
      "name": "api.gw.lo.",
      "type": "A",
      "changetype": "REPLACE",
      "records": [
        {"content": "192.168.1.201", "disabled": false},
        {"content": "192.168.1.202", "disabled": false},
        {"content": "192.168.1.203", "disabled": false},
        {"content": "192.168.1.200", "disabled": true},
        {"content": "192.168.1.168", "disabled": true}
      ],
      "comments": [{"content": "{\"type\":\"tcp\",\"port\":6443,\"timeout\":10}", "account": ""}],
      "ttl": 86400
    }]
  }'
```

**Current Status**: 3 active control plane nodes (192.168.1.201-203), 2 disabled nodes

#### Application Ingress Load Balancing with HTTP Health Checks

```bash
# OpenShift/Kubernetes worker nodes serving HTTP traffic on port 80
curl -X PATCH http://192.168.1.51:8081/api/v1/servers/localhost/zones/apps.gw.lo. \
  -H "X-API-Key: quest.5124" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [{
      "name": "*.apps.gw.lo.",
      "type": "A",
      "changetype": "REPLACE",
      "records": [
        {"content": "192.168.1.204", "disabled": false},
        {"content": "192.168.1.205", "disabled": false},
        {"content": "192.168.1.206", "disabled": false}
      ],
      "comments": [{"content": "{\"type\":\"tcp\",\"port\":80,\"timeout\":5}", "account": ""}],
      "ttl": 100
    }]
  }'
```

**Current Status**: 3 active worker nodes handling wildcard application routing

#### Mixed Health Check Types in Same Zone

```bash
# Individual control plane nodes with TCP health checks on API port
curl -X PATCH http://192.168.1.51:8081/api/v1/servers/localhost/zones/gw.lo. \
  -H "X-API-Key: quest.5124" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [
      {
        "name": "control0.gw.lo.",
        "type": "A",
        "changetype": "REPLACE",
        "records": [{"content": "192.168.1.201", "disabled": false}],
        "comments": [{"content": "{\"type\":\"tcp\",\"port\":6443,\"timeout\":10}", "account": ""}],
        "ttl": 86400
      },
      {
        "name": "worker0.gw.lo.",
        "type": "A",
        "changetype": "REPLACE",
        "records": [{"content": "192.168.1.204", "disabled": false}],
        "ttl": 86400
      }
    ]
  }'
```

**Result**: control0 gets TCP health checks, worker0 uses default ping health checks

## Service Management

```bash
# Development/testing
sudo /usr/local/bin/ploadb

# Production service management
sudo systemctl start ploadb
sudo systemctl status ploadb
sudo systemctl stop ploadb
sudo journalctl -u ploadb -f
```

## Change History

### Version 2.1 (2026-01-05) - Failsafe Mode
**Feature Addition**: Automatic failsafe when all hosts are unavailable

#### New Features
- **Failsafe Mode**: When all hosts in a load-balanced record fail health checks, the first entry is automatically enabled
- This ensures DNS queries always return at least one IP address, preventing complete service outages
- Failsafe activations are logged with "(failsafe)" suffix in state change messages

#### Implementation Changes
- `ploadb/ploadb.go:779-798`: Added failsafe logic in `handle_load_balance()` function
- After health checking all IPs, checks if all are disabled
- If all hosts unavailable, enables first entry and logs the failsafe activation

### Version 2.0 (2025-09-15) - Configurable Health Probes
**Major Feature Addition**: Multi-protocol health checking support

#### New Features
- **Configurable Probe Types**: Added support for ping, HTTP, and HTTPS health checks
- **Per-Record Configuration**: Health check settings stored in PowerDNS record comments as JSON
- **HTTP/HTTPS Probes**: Full support for custom paths, ports, timeouts, and expected status codes
- **Enhanced Logging**: Probe type now included in health state change log messages

#### Configuration Format
Health checks are configured using JSON in PowerDNS record comments:
```json
{"type": "http", "path": "/health", "port": 8080, "timeout": 10, "expected": 200}
```

#### Implementation Changes
- `ploadb/ploadb.go:47-53`: Added `ProbeConfig` struct for configuration parsing
- `ploadb/ploadb.go:72-99`: Added `parseProbeConfig()` function for JSON parsing
- `ploadb/ploadb.go:101-152`: Added probe implementation functions:
  - `performPingProbe()` - ICMP ping health checks
  - `performHTTPProbe()` - HTTP/HTTPS health checks
  - `performProbe()` - Main probe dispatcher
- `ploadb/ploadb.go:240-276`: Complete rewrite of `handle_load_balance()` function
- Added imports: `net/http`, `crypto/tls`, `encoding/json`, `strings`

#### Backward Compatibility
- Existing configurations continue to work unchanged
- Records without comments default to ping probes
- No configuration file changes required
- ICMP ping behavior preserved (3 packets, considers healthy if any received)

#### Technical Details
- **HTTP Probes**: Standard GET requests with configurable timeouts
- **HTTPS Probes**: Certificate verification disabled for operational simplicity
- **Error Handling**: Failed probe configurations fall back to ping probes
- **Performance**: Removed concurrent ping goroutines in favor of sequential probing per record

### Version 1.0 (Original) - Basic ICMP Ping Health Checks
**Initial Implementation**: DNS load balancing with ICMP ping health checks

#### Core Features
- PowerDNS API integration for zone and record management
- ICMP ping health checking (3 packets per IP)
- Automatic DNS record enable/disable based on health status
- 20-second health check intervals
- Systemd service integration
- Log rotation support

#### Architecture
- Single Go binary design
- Concurrent zone processing
- PowerDNS HTTP API communication
- TOML configuration file support