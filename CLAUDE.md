# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

PowerDNS Load Balancer (ploadb) is a DNS-based load balancer service that monitors multiple IP addresses for DNS A records and automatically enables/disables them based on ICMP ping health checks. It integrates with PowerDNS via its HTTP API to provide automatic failover and load distribution at the DNS level.

## Core Architecture

The service operates as a single Go binary (`ploadb/ploadb.go`) that runs as a Linux systemd service. It follows this flow:
- **Main Loop**: Every 20 seconds, queries PowerDNS API for all zones
- **Zone Processing**: For each zone, identifies A records with multiple IP addresses
- **Health Checking**: Performs concurrent ICMP pings (3 packets per IP, 5-second timeout)
- **DNS Updates**: Updates PowerDNS records to disable unreachable hosts via PATCH API calls

Key functions:
- `DoWork()` - Main processing loop (20-second intervals)
- `process_domain(domain)` - Processes individual zones
- `handle_load_balance()` - Performs health checks and updates records
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

Configuration is managed via TOML file at `/etc/ploadb.conf`:
```toml
Baseurl = "http://localhost:8081"        # PowerDNS API URL
ApiPassword = "your-api-key-here"        # PowerDNS API key
```

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

- **Concurrency**: Uses goroutines for concurrent zone processing and ping operations
- **Error Handling**: Basic error handling with log output; service continues on API/network errors
- **State Management**: Uses PowerDNS record `disabled` field (false=healthy, true=unhealthy)
- **Logging**: Writes to `/var/log/ploadb/ploadb.log` with automatic rotation (5MB max, 3 backups, 28-day retention)
- **Timing**: Hard-coded 20-second intervals, 3-packet pings, 5-second timeouts
- **Privileges**: Requires root or `cap_net_raw` capability for ICMP ping

## PowerDNS Integration

The service communicates with PowerDNS HTTP API using these endpoints:
- `GET /api/v1/servers/localhost/zones` - List zones
- `GET /api/v1/servers/localhost/zones/{zone}` - Get zone details
- `PATCH /api/v1/servers/localhost/zones/{zone}` - Update records

Authentication via `X-API-Key` header matching PowerDNS `api-key` configuration.

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