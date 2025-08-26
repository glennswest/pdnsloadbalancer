# Architecture & API Reference - PowerDNS Load Balancer

This document provides detailed technical information about the PowerDNS Load Balancer architecture, code structure, and API integration.

## System Architecture

### High-Level Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                      ploadb Service                             │
├─────────────────────────────────────────────────────────────────┤
│  main() → service.New() → program.Start() → program.run()      │
│     │                                           │               │
│     ├─ Logging Setup (lumberjack)               └─ DoWork()     │
│     ├─ Service Configuration                        │           │
│     └─ Service Control                              ▼           │
│                                            ┌─────────────────┐  │
│                                            │   Main Loop     │  │
│                                            │  (20s interval) │  │
│                                            └─────────────────┘  │
│                                                     │           │
│                                                     ▼           │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ getdomainlist() │  │ process_domain()│  │handle_load_     │ │
│  │                 │  │                 │  │balance()        │ │
│  │ • API Call      │  │ • getdomain()   │  │ • ICMP Ping     │ │
│  │ • Zone List     │  │ • Parse RRsets  │  │ • State Check   │ │
│  │                 │  │ • Filter A Recs │  │ • send_update() │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                     │
                                     ▼
        ┌─────────────────────────────────────────────────────────┐
        │                PowerDNS HTTP API                       │
        │                                                        │
        │  GET /api/v1/servers/localhost/zones                   │
        │  GET /api/v1/servers/localhost/zones/{zone}            │
        │  PATCH /api/v1/servers/localhost/zones/{zone}          │
        └─────────────────────────────────────────────────────────┘
                                     │
                                     ▼
        ┌─────────────────────────────────────────────────────────┐
        │                   Target Hosts                         │
        │                                                        │
        │  192.168.1.200 ◄─── ICMP Ping (3 packets)             │
        │  192.168.1.201 ◄─── Health Check                       │
        │  192.168.1.202 ◄─── Concurrent                         │
        │  192.168.1.203                                         │
        └─────────────────────────────────────────────────────────┘
```

### Component Breakdown

#### 1. Service Management (`main()`)
- **Purpose**: Initialize service, configure logging, handle service commands
- **Dependencies**: `github.com/kardianos/service`, `gopkg.in/natefinch/lumberjack.v2`
- **Key Functions**:
  - Service registration and control
  - Log rotation setup
  - Configuration loading

#### 2. Configuration Management (`ReadConfig()`)
- **Purpose**: Load and validate TOML configuration
- **File Location**: `/etc/ploadb.conf`
- **Dependencies**: `github.com/BurntSushi/toml`
- **Configuration Structure**:
  ```go
  type Config struct {
      Baseurl     string  // PowerDNS API URL
      ApiPassword string  // API authentication key
  }
  ```

#### 3. Main Control Loop (`DoWork()`)
- **Purpose**: Orchestrate zone monitoring and health checking
- **Interval**: 20 seconds (configurable)
- **Flow**:
  1. Get all zones from PowerDNS
  2. Process each zone concurrently
  3. Sleep for interval
  4. Repeat

#### 4. Zone Processing (`process_domain()`)
- **Purpose**: Analyze zone records for load balancing candidates
- **Logic**:
  - Parse zone data from PowerDNS API
  - Filter for A records with multiple IPs
  - Launch health checking goroutines

#### 5. Health Checking (`handle_load_balance()`)
- **Purpose**: Perform ICMP health checks and update DNS records
- **Dependencies**: `github.com/oilbeater/go-ping`
- **Process**:
  - Create ping instances for each IP
  - Execute pings concurrently
  - Evaluate health status changes
  - Update DNS records if needed

#### 6. DNS Updates (`send_update()`)
- **Purpose**: Push record changes to PowerDNS
- **Dependencies**: `github.com/go-resty/resty`, `github.com/tidwall/sjson`
- **Method**: PATCH requests to PowerDNS API

## Code Structure

### File Organization

```
pdnsloadbalancer/
├── ploadb/
│   ├── ploadb.go           # Main application code
│   ├── etc/
│   │   └── ploadb.conf     # Configuration file
│   └── ploadb              # Compiled binary
├── etc/systemd/system/
│   └── ploadb.service      # Systemd service definition
├── go.mod                  # Go module dependencies
├── go.sum                  # Dependency checksums
├── *.sh                    # Testing and utility scripts
└── *.json                  # Example data structures
```

### Core Data Structures

#### Configuration
```go
type Config struct {
    Baseurl     string `toml:"Baseurl"`
    ApiPassword string `toml:"ApiPassword"`
}
```

#### Service Interface
```go
type program struct{}

func (p *program) Start(s service.Service) error
func (p *program) run()
func (p *program) Stop(s service.Service) error
```

### Function Reference

#### API Communication Functions

**`getdomainlist() string`**
- **Purpose**: Retrieve all DNS zones from PowerDNS
- **HTTP Method**: GET
- **Endpoint**: `/api/v1/servers/localhost/zones`
- **Returns**: JSON string containing zone list

**`getdomain(domain string) string`**
- **Purpose**: Get detailed zone information
- **HTTP Method**: GET
- **Endpoint**: `/api/v1/servers/localhost/zones/{domain}`
- **Parameters**: `domain` - zone name
- **Returns**: JSON string with zone details

**`send_update(domain, name, records string) string`**
- **Purpose**: Update DNS records in PowerDNS
- **HTTP Method**: PATCH
- **Endpoint**: `/api/v1/servers/localhost/zones/{domain}`
- **Parameters**:
  - `domain` - zone name
  - `name` - record name
  - `records` - JSON record data
- **Returns**: API response

#### Health Checking Functions

**`handle_load_balance(domain, name string, count int, records string)`**
- **Purpose**: Perform health checks and manage record states
- **Parameters**:
  - `domain` - DNS zone
  - `name` - record name
  - `count` - number of IP addresses
  - `records` - JSON record data
- **Process**:
  1. Create ping instances
  2. Execute concurrent pings
  3. Evaluate results
  4. Update record states

#### Processing Functions

**`process_domain(domain string)`**
- **Purpose**: Process a single DNS zone
- **Logic**:
  - Get zone data
  - Parse RRsets
  - Filter A records with multiple IPs
  - Launch health checking goroutines

**`DoWork()`**
- **Purpose**: Main processing loop
- **Behavior**:
  - Infinite loop with 20-second intervals
  - Get all zones
  - Process each zone
  - Handle errors gracefully

## API Integration Details

### PowerDNS REST API

#### Authentication
```http
X-API-Key: your-api-key-here
Content-Type: application/json
```

#### Endpoints Used

**1. List Zones**
```http
GET /api/v1/servers/localhost/zones
```
Response:
```json
[
  {
    "name": "example.com.",
    "type": "Zone",
    "url": "/api/v1/servers/localhost/zones/example.com."
  }
]
```

**2. Get Zone Details**
```http
GET /api/v1/servers/localhost/zones/example.com.
```
Response:
```json
{
  "name": "example.com.",
  "type": "Zone",
  "rrsets": [
    {
      "name": "api.example.com.",
      "type": "A",
      "ttl": 300,
      "records": [
        {"content": "192.168.1.10", "disabled": false},
        {"content": "192.168.1.11", "disabled": false}
      ]
    }
  ]
}
```

**3. Update Zone Records**
```http
PATCH /api/v1/servers/localhost/zones/example.com.
```
Request Body:
```json
{
  "rrsets": [
    {
      "name": "api.example.com.",
      "type": "A",
      "changetype": "replace",
      "records": [
        {"content": "192.168.1.10", "disabled": false},
        {"content": "192.168.1.11", "disabled": true}
      ]
    }
  ]
}
```

### JSON Processing

The service uses `tidwall/gjson` and `tidwall/sjson` for efficient JSON manipulation:

**Reading JSON Data:**
```go
// Extract zone names
zones := gjson.Get(data, "#.name").String()

// Get record content
content := gjson.Get(record, "content").String()

// Check disabled state
disabled := gjson.Get(records, "records.0.disabled").String()
```

**Modifying JSON Data:**
```go
// Set disabled state
records, _ = sjson.SetRaw(records, "records.0.disabled", "true")

// Create update payload
data, _ := sjson.SetRaw("", "rrsets.0", records)
data, _ = sjson.Set(data, "rrsets.0.changetype", "replace")
```

## Health Checking Implementation

### ICMP Ping Process

```go
// Create ping instances for each IP
pger := []*ping.Pinger{}
for _, host := range recs.Array() {
    ip := gjson.Get(host.String(), "content").String()
    pg, _ := ping.NewPinger(ip)
    pg.SetPrivileged(true)  // Requires root or capabilities
    pg.Count = 3            // Send 3 packets
    pger = append(pger, pg)
    go pg.Run()             // Execute concurrently
}

// Wait for ping completion
time.Sleep(5 * time.Second)

// Process results
for idx, pg := range pger {
    stats := pg.Statistics()
    if stats.PacketsRecv > 0 {
        // Host is alive
        records = sjson.SetRaw(records, dsname, "false")
    } else {
        // Host is down
        records = sjson.SetRaw(records, dsname, "true") 
    }
}
```

### State Management

The service tracks health states using the `disabled` field in DNS records:
- `disabled: false` - Host is healthy, include in DNS responses
- `disabled: true` - Host is unhealthy, exclude from DNS responses

State changes trigger DNS updates only when actual changes occur.

## Concurrency Model

### Goroutine Usage

1. **Service Goroutine**: Main service runner
2. **Work Goroutine**: DoWork() infinite loop
3. **Zone Processing**: One goroutine per zone (via `process_domain`)
4. **Health Checking**: One goroutine per multi-IP A record
5. **Ping Execution**: One goroutine per IP address

### Thread Safety

The service uses minimal shared state:
- Configuration is read-only after initialization
- Each goroutine operates on independent data
- No explicit synchronization required for current implementation

## Error Handling

### API Errors

```go
// API calls use basic error handling
resp, err := client.R().Get("/api/v1/servers/localhost/zones")
if err != nil {
    // Error is logged but processing continues
    log.Printf("API Error: %v", err)
}
```

### Network Errors

Ping failures are treated as host unavailability rather than errors:
```go
if stats.PacketsRecv > 0 {
    // Host responsive
} else {
    // Host unresponsive - disable record
}
```

### Configuration Errors

```go
_, err := os.Stat(configfile)
if err != nil {
    log.Printf("Config file is missing: %s", configfile)
    return MyConfig  // Return empty config, continue operation
}
```

## Performance Characteristics

### Timing Intervals

- **Main Loop**: 20 seconds between zone scans
- **Ping Timeout**: 5 seconds wait for responses
- **Ping Count**: 3 packets per IP address
- **Concurrent Processing**: All pings execute simultaneously

### Scalability Factors

- **Zone Count**: Linear scaling with number of zones
- **Record Count**: Linear scaling with monitored records
- **IP Count**: Linear scaling with IPs per record
- **Network Latency**: Affects ping timeout requirements

### Resource Usage

- **Memory**: Minimal, primarily for JSON data and ping statistics
- **CPU**: Low, mainly during ping operations
- **Network**: ICMP traffic + HTTP API calls
- **Disk**: Log files only (with rotation)

## Extension Points

### Adding New Health Checks

To implement additional health check methods:

1. **Create new checker function**:
```go
func handle_tcp_check(domain, name string, port int, records string) {
    // Implement TCP connection check
}
```

2. **Modify record processing**:
```go
// In process_domain(), add logic to detect different check types
if thetype == "A" && hasMultipleIPs(entries) {
    checkType := getCheckType(element) // Custom logic
    switch checkType {
    case "icmp":
        go handle_load_balance(domain, thename, cnt, element.String())
    case "tcp":
        go handle_tcp_check(domain, thename, port, element.String())
    }
}
```

### Configuration Extensions

To add new configuration options:

1. **Update Config struct**:
```go
type Config struct {
    Baseurl      string
    ApiPassword  string
    PingCount    int    `toml:"PingCount"`
    PingTimeout  int    `toml:"PingTimeout"`
    CheckInterval int   `toml:"CheckInterval"`
}
```

2. **Use in code**:
```go
pg.Count = MyConfig.PingCount
time.Sleep(time.Duration(MyConfig.CheckInterval) * time.Second)
```

### Monitoring Integration

To add metrics/monitoring:

1. **Add metrics dependencies**:
```go
import "github.com/prometheus/client_golang/prometheus"
```

2. **Create metrics**:
```go
var (
    healthCheckCounter = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ploadb_health_checks_total",
        },
        []string{"record", "status"},
    )
)
```

3. **Update health check function**:
```go
if stats.PacketsRecv > 0 {
    healthCheckCounter.WithLabelValues(name, "healthy").Inc()
} else {
    healthCheckCounter.WithLabelValues(name, "unhealthy").Inc()
}
```

## Security Considerations

### API Security

- API keys transmitted in HTTP headers
- Consider HTTPS for production deployments
- Limit API access via firewall rules
- Use strong, unique API keys

### Network Security

- ICMP ping requires elevated privileges
- Consider network segmentation
- Monitor for ping flood potential
- Implement rate limiting if needed

### Service Security

- Run with minimal required privileges
- Use systemd security features
- Protect configuration files
- Implement log access controls

This architecture documentation provides the technical foundation for understanding, maintaining, and extending the PowerDNS Load Balancer service. 