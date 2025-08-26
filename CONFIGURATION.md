# Configuration Reference - PowerDNS Load Balancer

This document provides detailed configuration information for the PowerDNS Load Balancer service.

## Configuration File

The main configuration file is located at `/etc/ploadb.conf` and uses TOML format.

### Basic Configuration

```toml
# PowerDNS Load Balancer Configuration File
# Location: /etc/ploadb.conf

# PowerDNS API Base URL
# This should point to your PowerDNS server's API endpoint
Baseurl = "http://localhost:8081"

# PowerDNS API Key
# Must match the api-key configured in PowerDNS
ApiPassword = "your-secure-api-key-here"
```

### Configuration Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `Baseurl` | string | Yes | None | PowerDNS API base URL including protocol and port |
| `ApiPassword` | string | Yes | None | PowerDNS API authentication key |

### Configuration Examples

#### Local PowerDNS Server
```toml
Baseurl = "http://localhost:8081"
ApiPassword = "local-dev-key-12345"
```

#### Remote PowerDNS Server
```toml
Baseurl = "http://dns.example.com:8081"
ApiPassword = "prod-api-key-secure-string"
```

#### PowerDNS with HTTPS
```toml
Baseurl = "https://dns.example.com:8443"
ApiPassword = "ssl-enabled-api-key"
```

## PowerDNS Configuration

The PowerDNS server must be properly configured to work with ploadb.

### Required PowerDNS Settings

Edit your PowerDNS configuration file (typically `/etc/powerdns/pdns.conf`):

```ini
# Enable HTTP API
webserver=yes
webserver-address=0.0.0.0
webserver-port=8081
webserver-password=

# Enable API functionality
api=yes
api-key=your-secure-api-key-here

# Set allowed hosts for API access
webserver-allow-from=127.0.0.1,192.168.0.0/16,10.0.0.0/8

# Optional: Enable CORS for web interface
api-readonly=no
webserver-allow-from=0.0.0.0/0
```

### PowerDNS API Key Security

- Use a strong, unique API key (minimum 32 characters)
- Include letters, numbers, and special characters
- Store securely and limit access
- Consider using environment variables or secrets management

Example strong API key generation:
```bash
openssl rand -base64 32
```

## DNS Record Configuration

### Supported Record Types

ploadb only monitors **A records** with multiple IP addresses:

- **Supported**: A records with 2+ IP addresses
- **Ignored**: AAAA, CNAME, MX, TXT, SRV, PTR, NS, SOA records
- **Ignored**: A records with only 1 IP address

### DNS Zone Structure

Example zone configuration that ploadb will monitor:

```json
{
  "name": "api.example.com.",
  "type": "A",
  "ttl": 300,
  "records": [
    {
      "content": "192.168.1.10",
      "disabled": false
    },
    {
      "content": "192.168.1.11", 
      "disabled": false
    },
    {
      "content": "192.168.1.12",
      "disabled": false
    }
  ]
}
```

### Record Requirements

For ploadb to manage a DNS record, it must meet these criteria:

1. **Type**: Must be an A record
2. **Multiple IPs**: Must have 2 or more IP addresses
3. **Zone Access**: Zone must be accessible via PowerDNS API
4. **Valid IPs**: All IP addresses must be valid IPv4 addresses

### Example Monitored Records

```bash
# These records will be monitored by ploadb:
api.example.com.      300   IN   A   192.168.1.10
api.example.com.      300   IN   A   192.168.1.11
api.example.com.      300   IN   A   192.168.1.12

web.example.com.      300   IN   A   10.0.1.20
web.example.com.      300   IN   A   10.0.1.21

# These records will be ignored:
single.example.com.   300   IN   A   192.168.1.100    # Only 1 IP
mail.example.com.     300   IN   MX  10 mail.example.com.  # Not A record
```

## Service Configuration

### Systemd Service

The systemd service file is located at `/etc/systemd/system/ploadb.service`:

```ini
[Unit]
Description=PowerDNSLoadBalancer
ConditionFileIsExecutable=/usr/local/bin/ploadb

[Service]
Type=simple
ExecStart=/usr/local/bin/ploadb

# Restart configuration
Restart=always
RestartSec=10
RuntimeMaxSec=1800

# Optional security settings
User=root
Group=root

# Environment file (optional)
EnvironmentFile=-/etc/sysconfig/ploadb

[Install]
WantedBy=multi-user.target
```

### Service Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `Type` | Service type for systemd | `simple` |
| `ExecStart` | Path to ploadb binary | `/usr/local/bin/ploadb` |
| `Restart` | Restart policy | `always` |
| `RestartSec` | Delay before restart | `10` seconds |
| `RuntimeMaxSec` | Maximum runtime before restart | `1800` seconds |
| `User/Group` | Run as specific user | `root` |

### Environment Variables

Optional environment file `/etc/sysconfig/ploadb`:

```bash
# Optional environment variables for ploadb service
PLOADB_CONFIG_FILE="/etc/ploadb.conf"
PLOADB_LOG_LEVEL="INFO"
```

## Logging Configuration

### Log File Location

Logs are written to: `/var/log/ploadb/ploadb.log`

### Log Rotation Settings

Built-in log rotation via lumberjack:

```go
// Configured in main() function
log.SetOutput(&lumberjack.Logger{
    Filename:   "/var/log/ploadb/ploadb.log",
    MaxSize:    5,        // megabytes
    MaxBackups: 3,        // keep 3 backup files
    MaxAge:     28,       // days
    Compress:   true,     // compress old log files
})
```

### Log Levels and Format

Currently supports INFO level logging with timestamp format:
```
2024/01/15 10:30:15 api.example.com. - 192.168.1.10 changed state to false
2024/01/15 10:30:35 api.example.com. - 192.168.1.10 changed state to true
```

## Timing Configuration

### Health Check Intervals

Default timing values (configured in source code):

| Parameter | Value | Description |
|-----------|-------|-------------|
| Zone scan interval | 20 seconds | How often to check all zones |
| Ping timeout | 5 seconds | Wait time for ping responses |
| Ping count | 3 packets | Number of ping packets per IP |
| Ping wait | 5 seconds | Additional wait after pings |

### Modifying Timing

To change timing values, modify the source code:

```go
// In DoWork() function - main loop interval
time.Sleep(20 * time.Second)  // Change zone scan frequency

// In handle_load_balance() function
pg.Count = 3                   // Change ping packet count
time.Sleep(5 * time.Second)    // Change ping timeout
```

After modification, rebuild and reinstall:
```bash
go build -o ploadb ploadb.go
sudo systemctl stop ploadb
sudo cp ploadb /usr/local/bin/
sudo systemctl start ploadb
```

## Advanced Configuration

### Multi-Instance Deployment

To run multiple ploadb instances (not recommended):

1. **Create separate config files**:
```bash
sudo cp /etc/ploadb.conf /etc/ploadb-instance2.conf
```

2. **Modify source code** to read different config file

3. **Create separate service files**:
```bash
sudo cp /etc/systemd/system/ploadb.service /etc/systemd/system/ploadb-instance2.service
```

### High Availability Setup

For redundant load balancer deployment:

1. **Deploy on multiple servers**
2. **Use shared PowerDNS backend**
3. **Implement leader election** (requires code modification)
4. **Monitor service health** with external tools

### Performance Tuning

#### System Limits

Adjust system limits for high-volume environments:

```bash
# Increase file descriptor limits
echo "ploadb soft nofile 65536" >> /etc/security/limits.conf
echo "ploadb hard nofile 65536" >> /etc/security/limits.conf
```

#### Network Optimization

For networks with many monitored hosts:

```bash
# Increase network buffer sizes
echo "net.core.rmem_max = 16777216" >> /etc/sysctl.conf
echo "net.core.wmem_max = 16777216" >> /etc/sysctl.conf
sudo sysctl -p
```

## Security Configuration

### File Permissions

Secure configuration files:

```bash
# Configuration file
sudo chown root:root /etc/ploadb.conf
sudo chmod 600 /etc/ploadb.conf

# Log directory
sudo chown root:root /var/log/ploadb
sudo chmod 755 /var/log/ploadb

# Binary
sudo chown root:root /usr/local/bin/ploadb
sudo chmod 755 /usr/local/bin/ploadb
```

### Network Security

Restrict PowerDNS API access:

```ini
# In pdns.conf - limit API access
webserver-allow-from=127.0.0.1,192.168.1.100

# Use authentication
api-key=very-secure-long-random-string-here
```

### Service Isolation

Run with limited privileges (requires code modification for config file access):

```ini
[Service]
User=ploadb
Group=ploadb
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/log/ploadb
ReadOnlyPaths=/etc/ploadb.conf
```

## Troubleshooting Configuration

### Configuration Validation

```bash
# Test configuration file syntax
toml_validator /etc/ploadb.conf

# Test PowerDNS API connectivity
curl -H "X-API-Key: your-api-key" http://localhost:8081/api/v1/servers

# Verify DNS zones are accessible
curl -H "X-API-Key: your-api-key" http://localhost:8081/api/v1/servers/localhost/zones
```

### Common Configuration Errors

1. **Invalid TOML syntax**:
```bash
# Check for missing quotes, invalid characters
sudo ploadb -config-test  # (would need implementation)
```

2. **Wrong API endpoint**:
```bash
# Test endpoint manually
telnet dns-server 8081
```

3. **API key mismatch**:
```bash
# Compare keys between files
grep api-key /etc/powerdns/pdns.conf
grep ApiPassword /etc/ploadb.conf
```

4. **Permission issues**:
```bash
# Check file access
sudo -u ploadb cat /etc/ploadb.conf
```

### Configuration Testing

Before production deployment:

1. **Test with minimal zone**
2. **Verify logging works**
3. **Test health check accuracy**
4. **Validate DNS updates**
5. **Monitor resource usage**

This configuration reference should help you properly set up and tune the PowerDNS Load Balancer for your environment. 