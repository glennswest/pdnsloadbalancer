# Quick Start Guide - PowerDNS Load Balancer

Get up and running with the PowerDNS Load Balancer (`ploadb`) in under 10 minutes!

## Prerequisites Checklist

- [ ] Linux system with systemd
- [ ] PowerDNS server running
- [ ] Go 1.24+ installed
- [ ] Root/sudo access
- [ ] Basic DNS knowledge

## 5-Minute Setup

### Step 1: Enable PowerDNS API (2 minutes)

Edit `/etc/powerdns/pdns.conf`:
```ini
webserver=yes
webserver-port=8081
api=yes
api-key=test-key-12345
webserver-allow-from=127.0.0.1,192.168.0.0/16
```

Restart PowerDNS:
```bash
sudo systemctl restart pdns
```

Test API:
```bash
curl -H 'X-API-Key: test-key-12345' http://localhost:8081/api/v1/servers/localhost/zones
```

### Step 2: Build and Install ploadb (2 minutes)

```bash
# Build
cd pdnsloadbalancer/ploadb
go build -o ploadb ploadb.go

# Install
sudo cp ploadb /usr/local/bin/
sudo chmod +x /usr/local/bin/ploadb
sudo setcap cap_net_raw=+ep /usr/local/bin/ploadb
```

### Step 3: Configure ploadb (1 minute)

Create config file:
```bash
sudo tee /etc/ploadb.conf << EOF
Baseurl = "http://localhost:8081"
ApiPassword = "test-key-12345"
EOF

sudo chmod 600 /etc/ploadb.conf
sudo mkdir -p /var/log/ploadb
```

### Step 4: Create Test DNS Records

Add test records with multiple IPs to your PowerDNS zone:
```bash
curl -X PATCH -H 'X-API-Key: test-key-12345' \
     -H 'Content-Type: application/json' \
     -d '{
       "rrsets": [{
         "name": "test.yourdomain.com.",
         "type": "A",
         "changetype": "replace",
         "records": [
           {"content": "8.8.8.8", "disabled": false},
           {"content": "1.1.1.1", "disabled": false},
           {"content": "192.0.2.1", "disabled": false}
         ]
       }]
     }' \
     http://localhost:8081/api/v1/servers/localhost/zones/yourdomain.com.
```

### Step 5: Start ploadb

Run manually for testing:
```bash
sudo /usr/local/bin/ploadb
```

## Verification (30 seconds)

Watch the logs:
```bash
sudo tail -f /var/log/ploadb/ploadb.log
```

Expected output:
```
2024/01/15 10:30:15 test.yourdomain.com. - 8.8.8.8 changed state to false
2024/01/15 10:30:16 test.yourdomain.com. - 1.1.1.1 changed state to false  
2024/01/15 10:30:17 test.yourdomain.com. - 192.0.2.1 changed state to true
```

Query DNS to see active records:
```bash
nslookup test.yourdomain.com
```

## Production Setup

### Install as System Service

```bash
# Update service file paths
sudo tee /etc/systemd/system/ploadb.service << EOF
[Unit]
Description=PowerDNSLoadBalancer
ConditionFileIsExecutable=/usr/local/bin/ploadb

[Service]
Type=simple
ExecStart=/usr/local/bin/ploadb
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable ploadb
sudo systemctl start ploadb

# Check status
sudo systemctl status ploadb
```

## Testing Load Balancing

### Test Host Failure

1. **Block one IP** (simulates host failure):
```bash
sudo iptables -A OUTPUT -d 8.8.8.8 -p icmp -j DROP
```

2. **Watch logs** (should show state change):
```bash
sudo tail -f /var/log/ploadb/ploadb.log
```

3. **Test DNS resolution** (blocked IP should be excluded):
```bash
for i in {1..5}; do nslookup test.yourdomain.com; sleep 1; done
```

4. **Restore connectivity**:
```bash
sudo iptables -D OUTPUT -d 8.8.8.8 -p icmp -j DROP
```

5. **Verify restoration** (IP should be re-enabled):
```bash
sudo tail -f /var/log/ploadb/ploadb.log
```

## Real-World Example

### Multi-Server Web Service

Create load-balanced web service record:
```bash
curl -X PATCH -H 'X-API-Key: test-key-12345' \
     -H 'Content-Type: application/json' \
     -d '{
       "rrsets": [{
         "name": "web.example.com.",
         "type": "A", 
         "changetype": "replace",
         "ttl": 60,
         "records": [
           {"content": "192.168.1.10", "disabled": false},
           {"content": "192.168.1.11", "disabled": false},
           {"content": "192.168.1.12", "disabled": false}
         ]
       }]
     }' \
     http://localhost:8081/api/v1/servers/localhost/zones/example.com.
```

This creates a load-balanced DNS record where:
- Clients get different IPs in round-robin fashion
- Failed servers are automatically removed
- Recovered servers are automatically re-added
- TTL is set low (60s) for faster failover

## Monitoring

### Essential Commands

```bash
# Service status
sudo systemctl status ploadb

# Real-time logs
sudo journalctl -u ploadb -f

# Log file
sudo tail -f /var/log/ploadb/ploadb.log

# Test API connectivity
curl -H 'X-API-Key: your-key' http://localhost:8081/api/v1/servers

# List monitored zones
curl -H 'X-API-Key: your-key' http://localhost:8081/api/v1/servers/localhost/zones
```

### Health Dashboard

Create simple monitoring script:
```bash
#!/bin/bash
# save as /usr/local/bin/ploadb-status

echo "=== ploadb Status ==="
systemctl is-active ploadb
echo
echo "=== Recent Activity ==="
tail -n 10 /var/log/ploadb/ploadb.log
echo
echo "=== PowerDNS API ==="
curl -s -H 'X-API-Key: test-key-12345' \
     http://localhost:8081/api/v1/servers | \
     grep -q "localhost" && echo "API OK" || echo "API ERROR"
```

Make executable and run:
```bash
sudo chmod +x /usr/local/bin/ploadb-status
ploadb-status
```

## Common Issues & Quick Fixes

### Issue: "Permission denied" for ping
```bash
sudo setcap cap_net_raw=+ep /usr/local/bin/ploadb
```

### Issue: "Cannot connect to PowerDNS API"
```bash
# Test API manually
curl -v -H 'X-API-Key: test-key-12345' http://localhost:8081/api/v1/servers

# Check PowerDNS is running
sudo systemctl status pdns
```

### Issue: "No records being monitored" 
- Ensure DNS records have **multiple IP addresses**
- Only **A records** are monitored (not AAAA, CNAME, etc.)
- Check zone configuration in PowerDNS

### Issue: "Service fails to start"
```bash
# Check logs
sudo journalctl -u ploadb --no-pager

# Test manual execution
sudo /usr/local/bin/ploadb

# Verify config file
sudo cat /etc/ploadb.conf
```

## Production Checklist

Before going live:

- [ ] **Security**: Change default API keys
- [ ] **Firewall**: Restrict PowerDNS API access
- [ ] **Monitoring**: Set up log monitoring/alerting
- [ ] **Backup**: Backup PowerDNS configuration
- [ ] **Testing**: Verify failover behavior
- [ ] **Documentation**: Document your DNS zones
- [ ] **TTL**: Set appropriate DNS TTL values
- [ ] **Networking**: Ensure ICMP is allowed to all monitored hosts

## Next Steps

- Read [INSTALLATION.md](INSTALLATION.md) for detailed setup instructions
- Review [CONFIGURATION.md](CONFIGURATION.md) for advanced configuration
- Check [ARCHITECTURE.md](ARCHITECTURE.md) for technical details
- See [README.md](README.md) for comprehensive documentation

## Support

If you encounter issues:

1. **Check logs**: `/var/log/ploadb/ploadb.log`
2. **Verify connectivity**: Test PowerDNS API and ICMP to monitored hosts  
3. **Review configuration**: Ensure proper TOML syntax and valid values
4. **Test manually**: Run ploadb in foreground to see immediate output

---

🎉 **Congratulations!** You now have a working DNS-based load balancer that automatically manages host availability. Your DNS queries will automatically exclude failed hosts and include them when they recover. 