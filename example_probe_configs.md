# PowerDNS Load Balancer Probe Configuration Examples

The updated ploadb now supports configurable liveness probes per DNS entry using record comments in JSON format.

## Probe Types

### 1. Ping Probe (Default)
Default behavior if no comment is provided, or explicit configuration:
```json
{"type": "ping", "timeout": 5}
```

### 2. HTTP Probe
```json
{"type": "http", "path": "/health", "port": 8080, "timeout": 10, "expected": 200}
```

### 3. HTTPS Probe
```json
{"type": "https", "path": "/api/status", "port": 443, "timeout": 5, "expected": 200}
```

## Configuration Parameters

- `type`: "ping", "http", or "https" (default: "ping")
- `path`: HTTP(S) path to check (default: "/")
- `port`: Port number (defaults: 80 for http, 443 for https)
- `timeout`: Timeout in seconds (default: 5)
- `expected`: Expected HTTP status code (default: 200)

## PowerDNS Record Example

To configure an A record with HTTP health checks:

1. Add the A record with multiple IP addresses
2. Set the record comment to the JSON configuration

Example using PowerDNS API:
```bash
curl -X PATCH http://localhost:8081/api/v1/servers/localhost/zones/example.com \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [{
      "name": "app.example.com",
      "type": "A",
      "changetype": "REPLACE",
      "records": [
        {"content": "10.0.1.10", "disabled": false},
        {"content": "10.0.1.11", "disabled": false}
      ],
      "comment": "{\"type\":\"http\",\"path\":\"/health\",\"port\":8080,\"timeout\":10,\"expected\":200}"
    }]
  }'
```

## Behavior

- **Ping probes**: Send 3 ICMP packets, consider healthy if any packet is received
- **HTTP/HTTPS probes**: Make GET request to specified path, consider healthy if response status matches expected code
- **HTTPS probes**: Skip certificate verification for simplicity
- **Logging**: Probe type is now logged when health state changes
- **Fallback**: Invalid or missing configuration defaults to ping probe