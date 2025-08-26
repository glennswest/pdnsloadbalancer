# Installation Guide - PowerDNS Load Balancer

This guide provides detailed installation instructions for setting up the PowerDNS Load Balancer (`ploadb`) service.

## Prerequisites

### System Requirements

- **Operating System**: Linux with systemd support (tested on CentOS, Ubuntu, Debian)
- **Go Version**: 1.24.2 or later
- **Privileges**: Root access or sudo privileges
- **Network**: ICMP ping capabilities (raw socket permissions)

### PowerDNS Requirements

- PowerDNS server with API enabled
- PowerDNS version 4.x or later recommended
- Network connectivity between ploadb service and PowerDNS API

## Step 1: PowerDNS Configuration

### 1.1 Enable PowerDNS API

Edit your PowerDNS configuration file (usually `/etc/powerdns/pdns.conf`):

```ini
# Enable the built-in webserver
webserver=yes
webserver-address=0.0.0.0
webserver-port=8081

# Enable the API
api=yes
api-key=your-secure-api-key-here

# Allow API access (adjust IP ranges for your environment)
webserver-allow-from=127.0.0.1,192.168.0.0/16,10.0.0.0/8

# Optional: Enable CORS for web management
webserver-allow-from=0.0.0.0/0
api-readonly=no
```

### 1.2 Restart PowerDNS

```bash
sudo systemctl restart pdns
```

### 1.3 Verify API Access

```bash
curl -H 'X-API-Key: your-secure-api-key-here' \
     http://localhost:8081/api/v1/servers/localhost/zones
```

Expected response: JSON array of zones

## Step 2: Install Go (if not already installed)

### 2.1 Download and Install Go

```bash
# Download Go (adjust version as needed)
wget https://golang.org/dl/go1.24.2.linux-amd64.tar.gz

# Extract to /usr/local
sudo tar -C /usr/local -xzf go1.24.2.linux-amd64.tar.gz

# Add to PATH (add to ~/.bashrc for persistence)
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
```

### 2.2 Verify Go Installation

```bash
go version
```

## Step 3: Build ploadb

### 3.1 Prepare Build Environment

```bash
# Create workspace
mkdir -p $HOME/go/src
cd $HOME/go/src

# Copy or clone project files
# (assuming you have the source code in pdnsloadbalancer directory)
```

### 3.2 Build the Binary

```bash
cd pdnsloadbalancer/ploadb
go mod download
go build -o ploadb ploadb.go
```

### 3.3 Verify Build

```bash
./ploadb --help  # Should show usage information
```

## Step 4: Install Binary

### 4.1 Copy Binary to System Location

Choose one of the following installation paths:

**Option A: Standard system location (recommended)**
```bash
sudo cp ploadb /usr/local/bin/
sudo chmod +x /usr/local/bin/ploadb
```

**Option B: Custom location (update systemd service accordingly)**
```bash
sudo mkdir -p /root/go/src/github.com/ploadb
sudo cp ploadb /root/go/src/github.com/ploadb/
sudo chmod +x /root/go/src/github.com/ploadb/ploadb
```

### 4.2 Set Capabilities (if not running as root)

```bash
sudo setcap cap_net_raw=+ep /usr/local/bin/ploadb
```

## Step 5: Configuration

### 5.1 Create Configuration File

```bash
sudo mkdir -p /etc
sudo cp etc/ploadb.conf /etc/ploadb.conf
```

### 5.2 Edit Configuration

Edit `/etc/ploadb.conf`:

```toml
# PowerDNS Load Balancer Configuration

# PowerDNS API Base URL
Baseurl = "http://localhost:8081"

# PowerDNS API Key (must match pdns.conf api-key)
ApiPassword = "your-secure-api-key-here"
```

### 5.3 Secure Configuration File

```bash
sudo chown root:root /etc/ploadb.conf
sudo chmod 600 /etc/ploadb.conf
```

## Step 6: Logging Setup

### 6.1 Create Log Directory

```bash
sudo mkdir -p /var/log/ploadb
sudo chown root:root /var/log/ploadb
sudo chmod 755 /var/log/ploadb
```

### 6.2 Configure Log Rotation (Optional)

Create `/etc/logrotate.d/ploadb`:

```
/var/log/ploadb/*.log {
    daily
    missingok
    rotate 7
    compress
    notifempty
    create 644 root root
    postrotate
        /bin/systemctl reload ploadb > /dev/null 2>&1 || true
    endscript
}
```

## Step 7: Systemd Service Installation

### 7.1 Update Service File

Edit `etc/systemd/system/ploadb.service` and update the `ExecStart` path:

```ini
[Unit]
Description=PowerDNSLoadBalancer
ConditionFileIsExecutable=/usr/local/bin/ploadb

[Service]
Type=simple
RuntimeMaxSec=1800
ExecStart=/usr/local/bin/ploadb

Restart=always
RestartSec=10

# Optional: Run as dedicated user
# User=ploadb
# Group=ploadb

# Optional: Additional security
# PrivateTmp=yes
# ProtectSystem=strict
# ProtectHome=yes
# ReadWritePaths=/var/log/ploadb

EnvironmentFile=-/etc/sysconfig/ploadb

[Install]
WantedBy=multi-user.target
```

### 7.2 Install Service

```bash
sudo cp etc/systemd/system/ploadb.service /etc/systemd/system/
sudo systemctl daemon-reload
```

### 7.3 Enable and Start Service

```bash
sudo systemctl enable ploadb
sudo systemctl start ploadb
```

### 7.4 Verify Service Status

```bash
sudo systemctl status ploadb
sudo journalctl -u ploadb -f
```

## Step 8: Verification

### 8.1 Check Service is Running

```bash
sudo systemctl status ploadb
```

Expected output:
```
● ploadb.service - PowerDNSLoadBalancer
   Loaded: loaded (/etc/systemd/system/ploadb.service; enabled)
   Active: active (running) since ...
```

### 8.2 Monitor Logs

```bash
sudo tail -f /var/log/ploadb/ploadb.log
```

Expected output (with load-balanced records):
```
2024/01/15 10:30:15 Starting PowerDNS Load Balancer
2024/01/15 10:30:16 Processing zone: example.com.
2024/01/15 10:30:17 Monitoring: api.example.com. (4 IPs)
```

### 8.3 Test Health Checking

1. **Set up test DNS records** with multiple IPs:
```bash
# Use PowerDNS API to create test record
curl -X PATCH -H 'X-API-Key: your-api-key' \
     -H 'Content-Type: application/json' \
     -d '{
       "rrsets": [{
         "name": "test.example.com.",
         "type": "A",
         "changetype": "replace",
         "records": [
           {"content": "192.168.1.10", "disabled": false},
           {"content": "192.168.1.11", "disabled": false}
         ]
       }]
     }' \
     http://localhost:8081/api/v1/servers/localhost/zones/example.com.
```

2. **Block one IP** and watch logs for state changes:
```bash
# Block ICMP to 192.168.1.10
sudo iptables -A OUTPUT -d 192.168.1.10 -p icmp -j DROP

# Monitor logs
sudo tail -f /var/log/ploadb/ploadb.log
```

3. **Restore connectivity** and verify re-enabling:
```bash
# Unblock ICMP
sudo iptables -D OUTPUT -d 192.168.1.10 -p icmp -j DROP
```

## Troubleshooting Installation

### Common Issues

1. **Binary won't execute**
   ```bash
   # Check file permissions
   ls -l /usr/local/bin/ploadb
   
   # Check binary architecture
   file /usr/local/bin/ploadb
   ```

2. **Permission denied for ICMP**
   ```bash
   # Set capabilities
   sudo setcap cap_net_raw=+ep /usr/local/bin/ploadb
   
   # Or run as root (update systemd service)
   ```

3. **Cannot connect to PowerDNS API**
   ```bash
   # Test API connectivity
   curl -v -H 'X-API-Key: your-key' http://localhost:8081/api/v1/servers
   
   # Check PowerDNS configuration
   sudo systemctl status pdns
   ```

4. **Service fails to start**
   ```bash
   # Check systemd service status
   sudo systemctl status ploadb
   sudo journalctl -u ploadb --no-pager
   
   # Check configuration file
   sudo cat /etc/ploadb.conf
   
   # Test manual execution
   sudo /usr/local/bin/ploadb
   ```

### Performance Tuning

1. **Adjust timing intervals** (requires code modification):
   ```go
   // In DoWork() function
   time.Sleep(20 * time.Second)  // Adjust health check interval
   
   // In handle_load_balance() function
   time.Sleep(5 * time.Second)   // Adjust ping timeout
   pg.Count = 3                  // Adjust ping count
   ```

2. **Optimize systemd service**:
   ```ini
   # Add to service file for better performance
   [Service]
   Nice=-10
   IOSchedulingClass=1
   IOSchedulingPriority=4
   ```

## Security Hardening

### 1. Dedicated User

```bash
# Create dedicated user
sudo useradd -r -s /sbin/nologin ploadb
sudo mkdir -p /var/lib/ploadb
sudo chown ploadb:ploadb /var/lib/ploadb

# Update systemd service
# User=ploadb
# Group=ploadb
```

### 2. File Permissions

```bash
# Secure configuration
sudo chown root:root /etc/ploadb.conf
sudo chmod 600 /etc/ploadb.conf

# Secure binary
sudo chown root:root /usr/local/bin/ploadb
sudo chmod 755 /usr/local/bin/ploadb
```

### 3. Network Security

```bash
# Limit PowerDNS API access
# In pdns.conf:
# webserver-allow-from=127.0.0.1

# Use firewall to restrict access
sudo iptables -A INPUT -p tcp --dport 8081 -s 127.0.0.1 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8081 -j DROP
```

This completes the installation process. The service should now be monitoring your PowerDNS zones and automatically managing DNS record availability based on ICMP health checks. 