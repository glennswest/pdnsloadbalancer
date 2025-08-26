# PowerDNS Load Balancer (ploadb)

A DNS-based load balancer service that monitors multiple IP addresses for DNS A records and automatically enables/disables them based on ICMP ping health checks. This service integrates with PowerDNS via its HTTP API to provide automatic failover and load distribution at the DNS level.

## Overview

The PowerDNS Load Balancer (`ploadb`) is designed to run as a Linux systemd service alongside PowerDNS (pdns) and PowerDNS Recursor. It continuously monitors DNS zones for A records with multiple IP addresses, performs health checks via ICMP ping, and dynamically updates the DNS records to disable unreachable hosts and re-enable them when they come back online.

## Features

- **Automatic Health Monitoring**: ICMP ping-based health checks for all IP addresses in multi-IP A records
- **Dynamic DNS Updates**: Real-time enabling/disabling of DNS records based on host availability
- **PowerDNS Integration**: Uses PowerDNS HTTP API for seamless zone updates
- **Service Integration**: Runs as a proper Linux systemd service
- **Logging**: Comprehensive logging with automatic rotation
- **Configuration Management**: Simple TOML-based configuration
- **Concurrent Processing**: Handles multiple zones and records concurrently

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   ploadb        │    │    PowerDNS      │    │   DNS Clients   │
│   Service       │◄──►│    HTTP API      │◄──►│                 │
│                 │    │                  │    │                 │
│ • Health Checks │    │ • Zone Management│    │ • DNS Queries   │
│ • Record Updates│    │ • Record Storage │    │ • Load Balanced │
│ • Logging       │    │ • API Endpoints  │    │   Responses     │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                                              
         ▼                                              
┌─────────────────┐                                    
│  Target Hosts   │                                    
│                 │                                    
│ • IP: 192.168.1.200 ◄─── ICMP Ping                  
│ • IP: 192.168.1.201 ◄─── Health Checks              
│ • IP: 192.168.1.202 ◄─── Every 20s                  
│ • IP: 192.168.1.203                                  
└─────────────────┘                                    
```

## How It Works

1. **Zone Discovery**: Every 20 seconds, `ploadb` queries PowerDNS API to get all managed zones
2. **Record Analysis**: For each zone, it examines all A records and identifies those with multiple IP addresses
3. **Health Checking**: For each multi-IP A record, it performs concurrent ICMP ping tests (3 packets per IP)
4. **State Management**: Based on ping results, it determines if each IP should be enabled or disabled
5. **DNS Updates**: When state changes are detected, it updates the PowerDNS zone via API calls
6. **Logging**: All state changes and important events are logged with timestamps

## Installation

### Prerequisites

- PowerDNS with HTTP API enabled
- Go 1.24.2 or later
- Linux system with systemd
- Root or appropriate privileges for ICMP ping

### Building from Source

```bash
# Clone or copy the project files
cd /path/to/pdnsloadbalancer

# Build the binary
cd ploadb
go build -o ploadb ploadb.go

# Copy binary to appropriate location
sudo cp ploadb /usr/local/bin/
# or copy to the path specified in the systemd service file
sudo cp ploadb /root/go/src/github.com/ploadb/
```

### Configuration

1. **Create configuration file**:
```bash
sudo mkdir -p /etc
sudo cp ploadb/etc/ploadb.conf /etc/ploadb.conf
```

2. **Edit configuration** (`/etc/ploadb.conf`):
```toml
# PowerDNS API Configuration
Baseurl = "http://your-pdns-server:8081"
ApiPassword = "your-api-key-here"
```

3. **Set up logging directory**:
```bash
sudo mkdir -p /var/log/ploadb
sudo chown root:root /var/log/ploadb
sudo chmod 755 /var/log/ploadb
```

4. **Install systemd service**:
```bash
sudo cp etc/systemd/system/ploadb.service /etc/systemd/system/
sudo systemctl daemon-reload
```

### PowerDNS Configuration

Ensure PowerDNS is configured with API access enabled. In your `pdns.conf`:

```ini
# Enable the built-in webserver
webserver=yes
webserver-address=0.0.0.0
webserver-port=8081

# Enable the API
api=yes
api-key=your-api-key-here

# Allow API access (adjust IP ranges as needed)
webserver-allow-from=127.0.0.1,192.168.0.0/16
```

## Usage

### Service Management

```bash
# Enable and start the service
sudo systemctl enable ploadb
sudo systemctl start ploadb

# Check service status
sudo systemctl status ploadb

# View logs
sudo journalctl -u ploadb -f

# Stop the service
sudo systemctl stop ploadb

# Restart the service
sudo systemctl restart ploadb
```

### Manual Execution

For testing or debugging:

```bash
# Run in foreground
sudo /usr/local/bin/ploadb

# Install as service
sudo /usr/local/bin/ploadb install

# Start service
sudo /usr/local/bin/ploadb start

# Stop service
sudo /usr/local/bin/ploadb stop
```

### Monitoring and Logs

Logs are written to `/var/log/ploadb/ploadb.log` with automatic rotation:
- Maximum file size: 5 MB
- Keep 3 backup files
- Rotate files older than 28 days
- Compress old log files

Example log entries:
```
2024/01/15 10:30:15 api-int.gw.lo. - 192.168.1.201 changed state to false
2024/01/15 10:30:35 api-int.gw.lo. - 192.168.1.201 changed state to true
```

## Configuration Reference

### Configuration File (`/etc/ploadb.conf`)

| Parameter | Description | Example |
|-----------|-------------|---------|
| `Baseurl` | PowerDNS API base URL | `"http://localhost:8081"` |
| `ApiPassword` | PowerDNS API key | `"your-secret-api-key"` |

### DNS Record Requirements

For load balancing to work, DNS A records must have:
- **Multiple IP addresses** (2 or more)
- **Type A records only** (AAAA, CNAME, etc. are ignored)
- **Proper zone configuration** in PowerDNS

Example DNS zone configuration:
```json
{
  "name": "api.example.com.",
  "type": "A",
  "ttl": 300,
  "records": [
    {"content": "192.168.1.10", "disabled": false},
    {"content": "192.168.1.11", "disabled": false},
    {"content": "192.168.1.12", "disabled": false}
  ]
}
```

## Testing

### Test Scripts

The project includes several test scripts:

1. **`getit.sh`** - Retrieve specific zone information
2. **`getzones.sh`** - List all zones
3. **`test.sh`** - Test DNS updates and query resolution

### Manual Testing

```bash
# Test API connectivity
curl -H 'X-API-Key: your-api-key' http://your-pdns:8081/api/v1/servers/localhost/zones

# Test DNS resolution
nslookup your-load-balanced-record.example.com your-dns-server

# Monitor real-time logs
sudo tail -f /var/log/ploadb/ploadb.log
```

### Health Check Testing

To test the health checking mechanism:

1. **Simulate host failure**: Block ICMP on one of the target hosts
2. **Watch logs**: Monitor `/var/log/ploadb/ploadb.log` for state changes
3. **Verify DNS**: Query the DNS record to confirm the failed host is removed
4. **Restore connectivity**: Unblock ICMP and verify the host is re-enabled

## Troubleshooting

### Common Issues

1. **Permission Denied for ICMP**
   ```
   Solution: Run as root or set appropriate capabilities:
   sudo setcap cap_net_raw=+ep /usr/local/bin/ploadb
   ```

2. **API Connection Failed**
   ```
   Check: PowerDNS API configuration and network connectivity
   Test: curl -H 'X-API-Key: key' http://pdns-server:8081/api/v1/servers
   ```

3. **No Records Being Monitored**
   ```
   Verify: DNS records have multiple IP addresses and are type A
   Check: Zone configuration in PowerDNS
   ```

4. **Service Won't Start**
   ```
   Check: Binary path in systemd service file
   Verify: Configuration file exists and is readable
   Review: systemctl status ploadb and journalctl -u ploadb
   ```

### Debug Mode

Enable debug logging by uncommenting debug lines in the source code and rebuilding:

```go
// Uncomment these lines in ploadb.go for detailed output
fmt.Printf("Response Info: %s\n", resp.String())
fmt.Printf("Status Code: %d\n", resp.StatusCode())
```

## API Integration

### PowerDNS API Endpoints Used

- `GET /api/v1/servers/localhost/zones` - List all zones
- `GET /api/v1/servers/localhost/zones/{zone}` - Get zone details
- `PATCH /api/v1/servers/localhost/zones/{zone}` - Update zone records

### Data Structures

The service works with PowerDNS API JSON structures:

```json
{
  "rrsets": [{
    "name": "api.example.com.",
    "type": "A", 
    "changetype": "replace",
    "records": [
      {"content": "192.168.1.10", "disabled": false},
      {"content": "192.168.1.11", "disabled": true}
    ]
  }]
}
```

## Dependencies

### Go Modules

- `github.com/BurntSushi/toml` - Configuration file parsing
- `github.com/go-resty/resty` - HTTP client for API calls
- `github.com/kardianos/service` - Cross-platform service management
- `github.com/oilbeater/go-ping` - ICMP ping implementation
- `github.com/tidwall/gjson` - JSON parsing and querying
- `github.com/tidwall/sjson` - JSON modification
- `gopkg.in/natefinch/lumberjack.v2` - Log rotation

### System Requirements

- Linux with systemd
- PowerDNS server with API enabled
- Network connectivity to target hosts
- ICMP ping capabilities

## Security Considerations

- **API Key Protection**: Store PowerDNS API keys securely
- **Network Access**: Limit API access to trusted networks
- **Service Isolation**: Run service with minimal required privileges
- **Log Security**: Protect log files from unauthorized access

## Performance

### Timing Configuration

- **Health Check Interval**: 20 seconds (configurable in code)
- **Ping Count**: 3 packets per IP
- **Ping Timeout**: 5 seconds wait for responses
- **Concurrent Processing**: All pings executed in parallel

### Scalability

The service is designed to handle:
- Multiple DNS zones simultaneously
- Multiple A records per zone
- Multiple IP addresses per A record
- Concurrent health checks for all monitored IPs

## Contributing

To contribute to this project:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## License

[Specify your license here]

## Support

For issues and questions:
- Check the troubleshooting section above
- Review log files in `/var/log/ploadb/`
- Verify PowerDNS API connectivity
- Test network connectivity to monitored hosts 