package main

import "github.com/tidwall/sjson"
import "github.com/tidwall/gjson"
import "os"
import "github.com/go-resty/resty"
import "fmt"
import "time"
//import ping "github.com/sparrc/go-ping"
// Close raise condition fixed by using oilbeater
import ping "github.com/oilbeater/go-ping"
import "strconv"
import "log"
import "gopkg.in/natefinch/lumberjack.v2"
import "github.com/kardianos/service"
import "github.com/BurntSushi/toml"
import "net/http"
import "crypto/tls"
import "encoding/json"
import "strings"
import "net"
import "sync"
import "github.com/gorilla/websocket"
 

type program struct{}

func (p *program) Start(s service.Service) error {
        ReadConfig()
	startWebServer()
	go p.run()
	return nil
}
func (p *program) run() {
	// Do work here
        go DoWork()
}
func (p *program) Stop(s service.Service) error {
	// Stop should not block. Return with a few seconds.
	return nil
}

// Info from config file
type Config struct {
	Baseurl     string
	ApiPassword string
	WebPort     string
}

var MyConfig Config

// Web GUI data structures
type TargetStatus struct {
	Hostname    string    `json:"hostname"`
	IP          string    `json:"ip"`
	Enabled     bool      `json:"enabled"`
	ProbeType   string    `json:"probeType"`
	LastChecked time.Time `json:"lastChecked"`
	Zone        string    `json:"zone"`
}

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Hostname  string    `json:"hostname"`
	IP        string    `json:"ip"`
	OldState  string    `json:"oldState"`
	NewState  string    `json:"newState"`
	ProbeType string    `json:"probeType"`
	Message   string    `json:"message"`
}

type StatusData struct {
	Targets []TargetStatus `json:"targets"`
	Logs    []LogEntry     `json:"logs"`
}

// Global state for web GUI
var (
	statusMutex   sync.RWMutex
	currentStatus StatusData
	wsClients     = make(map[*websocket.Conn]bool)
	wsClientsMutex sync.RWMutex
	upgrader      = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for simplicity
		},
	}
)

type ProbeConfig struct {
	Type     string `json:"type"`     // "ping", "http", "https", or "tcp"
	Path     string `json:"path"`     // HTTP path (optional, default "/")
	Port     int    `json:"port"`     // Port (optional, defaults: 80 for http, 443 for https, required for tcp)
	Timeout  int    `json:"timeout"`  // Timeout in seconds (optional, default 5)
	Expected int    `json:"expected"` // Expected HTTP status code (optional, default 200)
}

// Reads info from config file
func ReadConfig() Config {
	var configfile = "/etc/ploadb.conf"
	_, err := os.Stat(configfile)
	if err != nil {
		log.Printf("Config file is missing: %s", configfile)
                return MyConfig
	}

	if _, err := toml.DecodeFile(configfile, &MyConfig); err != nil {
		log.Printf("Cannot Decode Config file %v\n",err)
                return MyConfig
	}
	//log.Print(MyConfig.Index)
	return MyConfig
}

func parseProbeConfig(comment string) ProbeConfig {
	config := ProbeConfig{
		Type:     "ping",
		Path:     "/",
		Port:     0,
		Timeout:  5,
		Expected: 200,
	}

	if comment == "" {
		return config
	}

	if strings.HasPrefix(comment, "{") {
		json.Unmarshal([]byte(comment), &config)
	}

	if config.Port == 0 {
		switch config.Type {
		case "http":
			config.Port = 80
		case "https":
			config.Port = 443
		}
	}

	return config
}

func performPingProbe(ip string, timeout int) bool {
	pg, err := ping.NewPinger(ip)
	if err != nil {
		log.Printf("Failed to create pinger for %s: %v", ip, err)
		return false
	}
	pg.SetPrivileged(true)
	pg.Count = 3
	pg.Timeout = time.Duration(timeout) * time.Second
	pg.Run()
	stats := pg.Statistics()
	return stats.PacketsRecv > 0
}

func performHTTPProbe(ip string, config ProbeConfig) bool {
	scheme := config.Type
	url := fmt.Sprintf("%s://%s:%d%s", scheme, ip, config.Port, config.Path)

	client := &http.Client{
		Timeout: time.Duration(config.Timeout) * time.Second,
	}

	if scheme == "https" {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Get(url)
	if err != nil {
		log.Printf("HTTP probe failed for %s: %v", url, err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == config.Expected
}

func performTCPProbe(ip string, config ProbeConfig) bool {
	if config.Port <= 0 || config.Port > 65535 {
		log.Printf("TCP probe failed for %s: invalid port %d", ip, config.Port)
		return false
	}

	address := fmt.Sprintf("%s:%d", ip, config.Port)
	timeout := time.Duration(config.Timeout) * time.Second

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		log.Printf("TCP probe failed for %s: %v", address, err)
		return false
	}
	defer conn.Close()

	return true
}

func performProbe(ip string, config ProbeConfig) bool {
	switch config.Type {
	case "http", "https":
		return performHTTPProbe(ip, config)
	case "tcp":
		return performTCPProbe(ip, config)
	case "ping":
		fallthrough
	default:
		return performPingProbe(ip, config.Timeout)
	}
}


func main() {
	// Start should not block. Do the actual work async.
        log.SetOutput(&lumberjack.Logger{
                                  Filename:   "/var/log/ploadb/ploadb.log",
                                  MaxSize:    5, // megabytes
                                  MaxBackups: 3,
                                  MaxAge:     28, //days
                                  Compress:   true, // disabled by default
                                 })
	svcConfig := &service.Config{
		Name:        "ploadb",
		DisplayName: "Monitor hostnames with multiple ip and disable when down,enable when up",
		Description: "PowerDNSLoadBalancer",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Printf("Fatal: %v\n",err)
                return
	}
        if len(os.Args) > 1 {
		err = service.Control(s, os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		return
	   }
	err = s.Run()
	if err != nil {
               log.Printf("Error: %v\n",err)
               return
	}
}


func getdomain(domain string) string{
      // Create a Resty Client
       client := resty.New()
       client.SetHostURL(MyConfig.Baseurl)
       resp, _ := client.R().
           SetHeaders(map[string]string{
                      "Content-Type": "application/json",
                       "X-API-KEY": MyConfig.ApiPassword}).
           Get("/api/v1/servers/localhost/zones/" + domain)
        // Explore response object
        /*
        fmt.Println("Response Info:")
        fmt.Println("Error      :", err)
        fmt.Println("Status Code:", resp.StatusCode())
        fmt.Println("Status     :", resp.Status())
        fmt.Println("Time       :", resp.Time())
        fmt.Println("Received At:", resp.ReceivedAt())
        fmt.Println("Body       :\n", resp)
        fmt.Println()
        */

        return(resp.String())
}

func getdomainlist() string{
      // Create a Resty Client
       client := resty.New()
       client.SetHostURL(MyConfig.Baseurl)
       resp, _ := client.R().
           SetHeaders(map[string]string{
                      "Content-Type": "application/json",
                       "X-API-KEY": MyConfig.ApiPassword}).
           Get("/api/v1/servers/localhost/zones")
        // Explore response object
        /*
        fmt.Println("Response Info:")
        //fmt.Println("Error      :", err)
        fmt.Println("Status Code:", resp.StatusCode())
        fmt.Println("Status     :", resp.Status())
        fmt.Println("Time       :", resp.Time())
        fmt.Println("Received At:", resp.ReceivedAt())
        fmt.Println("Body       :\n", resp)
        fmt.Println()
        */
        data := resp.String()
        zones := gjson.Get(data,"#.name").String()
        return(zones)
}

func handle_load_balance(domain string,name string,count int,records string){
       recs := gjson.Get(records,"records")
       changed := false

       // Get the record comment for probe configuration
       comment := gjson.Get(records, "comments.0.content").String()
       probeConfig := parseProbeConfig(comment)

       for idx, host := range(recs.Array()){
           ipa := gjson.Get(host.String(),"content")
           ip := ipa.String()

           // Perform the appropriate probe based on configuration
           isHealthy := performProbe(ip, probeConfig)

           dsname := "records." + strconv.Itoa(idx) + ".disabled"
           cstate := gjson.Get(records,dsname).String()

           // Update target status for web GUI
           updateTargetStatus(domain, name, ip, probeConfig.Type, isHealthy)

           if isHealthy {
               if (cstate == "true"){
                   log.Printf("%s - %s changed state from disabled to enabled (%s probe)", name, ip, probeConfig.Type)
                   addLogEntry(name, ip, "disabled", "enabled", probeConfig.Type)
                   changed = true
               }
               records, _ = sjson.SetRaw(records,dsname,"false")
           } else {
               if (cstate == "false"){
                   log.Printf("%s - %s changed state from enabled to disabled (%s probe)", name, ip, probeConfig.Type)
                   addLogEntry(name, ip, "enabled", "disabled", probeConfig.Type)
                   changed = true
               }
               records, _ = sjson.SetRaw(records,dsname,"true")
           }
       }

       if (changed == true){
           send_update(domain,name,records)
       }
}

func send_update(domain string,name string,records string) string{
// Create a Resty Client
       data, _  := sjson.SetRaw("","rrsets.0",records)
       data, _ = sjson.Set(data,"rrsets.0.changetype", "replace")
       fmt.Printf("send_update: %s\n",data)
       client := resty.New()
       client.SetHostURL(MyConfig.Baseurl)
       resp, _ := client.R().
           SetHeaders(map[string]string{
                      "Content-Type": "application/json",
                       "X-API-KEY": MyConfig.ApiPassword}).
           SetBody(data).
           Patch("/api/v1/servers/localhost/zones/" + domain)
        // Explore response object
        /*
        fmt.Println("Response Info:")
        //fmt.Println("Error      :", err)
        fmt.Println("Status Code:", resp.StatusCode())
        fmt.Println("Status     :", resp.Status())
        fmt.Println("Time       :", resp.Time())
        fmt.Println("Received At:", resp.ReceivedAt())
        fmt.Println("Body       :\n", resp)
        fmt.Println()
        */
        return(resp.String())


}

func DoWork(){

     for {
        domainsjs := getdomainlist()
        domains := gjson.Parse(domainsjs).Array()
        for _,domain := range domains{
              process_domain(domain.String())
              }
       time.Sleep(10 * time.Second)
        }
}

func process_domain(domain string){


        data := getdomain(domain)
        rrsets := gjson.Get(data,"rrsets").Array()
        for _,element := range rrsets{
             thename := gjson.Get(element.String(),"name").String()
             thetype := gjson.Get(element.String(),"type").String()
             entries := gjson.Get(element.String(),"records")
             cnt := len(entries.Array())
             if cnt > 1 && thetype != "" && thetype == "A"{
                go handle_load_balance(domain,thename,cnt,element.String())
                }
             }
}

// Web GUI HTML template
const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>PowerDNS Load Balancer - Status Dashboard</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            margin: 0;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            text-align: center;
        }
        .dashboard {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
            padding: 20px;
        }
        .panel {
            border: 1px solid #ddd;
            border-radius: 6px;
            overflow: hidden;
        }
        .panel-header {
            background: #f8f9fa;
            padding: 15px;
            font-weight: bold;
            border-bottom: 1px solid #ddd;
        }
        .panel-content {
            padding: 15px;
            max-height: 500px;
            overflow-y: auto;
        }
        .tree-view {
            font-family: monospace;
        }
        .zone {
            font-weight: bold;
            margin: 10px 0;
            color: #2c3e50;
            cursor: pointer;
        }
        .zone:hover {
            color: #3498db;
        }
        .hostname {
            margin-left: 20px;
            margin: 5px 0;
            padding: 5px;
            background: #f8f9fa;
            border-radius: 4px;
        }
        .target {
            margin-left: 40px;
            padding: 3px 8px;
            margin: 3px 0;
            border-radius: 3px;
            font-size: 0.9em;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .target.enabled {
            background: #d4edda;
            border-left: 3px solid #28a745;
        }
        .target.disabled {
            background: #f8d7da;
            border-left: 3px solid #dc3545;
        }
        .status-badge {
            padding: 2px 6px;
            border-radius: 12px;
            font-size: 0.8em;
            font-weight: bold;
        }
        .status-enabled {
            background: #28a745;
            color: white;
        }
        .status-disabled {
            background: #dc3545;
            color: white;
        }
        .probe-type {
            font-size: 0.7em;
            padding: 1px 4px;
            background: #6c757d;
            color: white;
            border-radius: 8px;
            margin-left: 5px;
        }
        .log-grid {
            border-collapse: collapse;
            width: 100%;
            font-size: 0.9em;
        }
        .log-grid th, .log-grid td {
            padding: 8px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        .log-grid th {
            background: #f8f9fa;
            position: sticky;
            top: 0;
            z-index: 1;
        }
        .log-entry.state-change {
            background: #fff3cd;
        }
        .timestamp {
            font-family: monospace;
            color: #6c757d;
            white-space: nowrap;
        }
        .connection-status {
            position: fixed;
            top: 10px;
            right: 10px;
            padding: 8px 12px;
            border-radius: 4px;
            font-size: 0.8em;
            font-weight: bold;
        }
        .connected {
            background: #d4edda;
            color: #155724;
        }
        .disconnected {
            background: #f8d7da;
            color: #721c24;
        }
        @media (max-width: 768px) {
            .dashboard {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>
<body>
    <div class="connection-status" id="connectionStatus">Disconnected</div>

    <div class="container">
        <div class="header">
            <h1>PowerDNS Load Balancer</h1>
            <p>Real-time Status Dashboard</p>
        </div>

        <div class="dashboard">
            <div class="panel">
                <div class="panel-header">Load Balanced Targets</div>
                <div class="panel-content">
                    <div id="targetTree" class="tree-view">
                        <div>Loading targets...</div>
                    </div>
                </div>
            </div>

            <div class="panel">
                <div class="panel-header">Status Change Log</div>
                <div class="panel-content">
                    <table class="log-grid" id="logTable">
                        <thead>
                            <tr>
                                <th>Time</th>
                                <th>Hostname</th>
                                <th>IP</th>
                                <th>Change</th>
                                <th>Probe</th>
                            </tr>
                        </thead>
                        <tbody id="logBody">
                            <tr><td colspan="5">Loading logs...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </div>

    <script>
        let ws = null;
        let reconnectInterval = null;

        function connect() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.host + '/ws';

            ws = new WebSocket(wsUrl);

            ws.onopen = function() {
                console.log('Connected to WebSocket');
                document.getElementById('connectionStatus').className = 'connection-status connected';
                document.getElementById('connectionStatus').textContent = 'Connected';
                clearInterval(reconnectInterval);
            };

            ws.onmessage = function(event) {
                const data = JSON.parse(event.data);
                updateTargets(data.targets);
                updateLogs(data.logs);
            };

            ws.onclose = function() {
                console.log('WebSocket connection closed');
                document.getElementById('connectionStatus').className = 'connection-status disconnected';
                document.getElementById('connectionStatus').textContent = 'Disconnected';
                reconnect();
            };

            ws.onerror = function(error) {
                console.error('WebSocket error:', error);
            };
        }

        function reconnect() {
            reconnectInterval = setInterval(() => {
                console.log('Attempting to reconnect...');
                connect();
            }, 5000);
        }

        function updateTargets(targets) {
            const targetTree = document.getElementById('targetTree');
            const zones = {};

            // Group targets by zone
            targets.forEach(target => {
                if (!zones[target.zone]) {
                    zones[target.zone] = {};
                }
                if (!zones[target.zone][target.hostname]) {
                    zones[target.zone][target.hostname] = [];
                }
                zones[target.zone][target.hostname].push(target);
            });

            let html = '';
            Object.keys(zones).sort().forEach(zone => {
                html += '<div class="zone">📁 ' + zone + '</div>';
                Object.keys(zones[zone]).sort().forEach(hostname => {
                    html += '<div class="hostname">🖥️ ' + hostname + '</div>';
                    zones[zone][hostname].sort((a, b) => {
                        // Sort IP addresses in ascending order (lowest first)
                        const ipA = a.ip.split('.').map(num => parseInt(num, 10).toString().padStart(3, '0')).join('.');
                        const ipB = b.ip.split('.').map(num => parseInt(num, 10).toString().padStart(3, '0')).join('.');
                        return ipA.localeCompare(ipB);
                    }).forEach(target => {
                        const statusClass = target.enabled ? 'enabled' : 'disabled';
                        const statusBadge = target.enabled ? 'ENABLED' : 'DISABLED';
                        const lastChecked = new Date(target.lastChecked).toLocaleTimeString();
                        html += '<div class="target ' + statusClass + '">';
                        html += '<span>📍 ' + target.ip + '</span>';
                        html += '<span>';
                        html += '<span class="status-badge status-' + statusClass + '">' + statusBadge + '</span>';
                        html += '<span class="probe-type">' + target.probeType + '</span>';
                        html += '</span>';
                        html += '</div>';
                    });
                });
            });

            if (html === '') {
                html = '<div>No load balanced targets found</div>';
            }

            targetTree.innerHTML = html;
        }

        function updateLogs(logs) {
            const logBody = document.getElementById('logBody');
            let html = '';

            [...logs].slice(-50).reverse().forEach(log => { // Show last 50 logs, newest first
                const timestamp = new Date(log.timestamp).toLocaleString();
                const changeText = log.oldState + ' → ' + log.newState;
                html += '<tr class="log-entry state-change">';
                html += '<td class="timestamp">' + timestamp + '</td>';
                html += '<td>' + log.hostname + '</td>';
                html += '<td>' + log.ip + '</td>';
                html += '<td>' + changeText + '</td>';
                html += '<td><span class="probe-type">' + log.probeType + '</span></td>';
                html += '</tr>';
            });

            if (html === '') {
                html = '<tr><td colspan="5">No status changes logged yet</td></tr>';
            }

            logBody.innerHTML = html;
        }

        // Start connection when page loads
        connect();
    </script>
</body>
</html>
`

// Web server functions
func startWebServer() {
	if MyConfig.WebPort == "" {
		MyConfig.WebPort = "8080" // Default port
	}

	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/debug", serveDebug)

	log.Printf("Starting web server on port %s", MyConfig.WebPort)
	go func() {
		err := http.ListenAndServe(":"+MyConfig.WebPort, nil)
		if err != nil {
			log.Printf("Web server error: %v", err)
		}
	}()
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlTemplate))
}

func serveDebug(w http.ResponseWriter, r *http.Request) {
	statusMutex.RLock()
	data, err := json.Marshal(currentStatus)
	statusMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	w.Write(data)
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	wsClientsMutex.Lock()
	wsClients[conn] = true
	wsClientsMutex.Unlock()

	// Send initial data
	statusMutex.RLock()
	data, _ := json.Marshal(currentStatus)
	statusMutex.RUnlock()
	conn.WriteMessage(websocket.TextMessage, data)

	// Keep connection alive and handle disconnect
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			wsClientsMutex.Lock()
			delete(wsClients, conn)
			wsClientsMutex.Unlock()
			break
		}
	}
}

func broadcastUpdate() {
	statusMutex.RLock()
	data, err := json.Marshal(currentStatus)
	statusMutex.RUnlock()

	if err != nil {
		log.Printf("Error marshaling status data: %v", err)
		return
	}

	wsClientsMutex.Lock()
	defer wsClientsMutex.Unlock()

	for conn := range wsClients {
		err := conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			conn.Close()
			delete(wsClients, conn)
		}
	}
}

func addLogEntry(hostname, ip, oldState, newState, probeType string) {
	statusMutex.Lock()

	logEntry := LogEntry{
		Timestamp: time.Now(),
		Hostname:  hostname,
		IP:        ip,
		OldState:  oldState,
		NewState:  newState,
		ProbeType: probeType,
		Message:   fmt.Sprintf("%s - %s changed state from %s to %s (%s probe)", hostname, ip, oldState, newState, probeType),
	}

	currentStatus.Logs = append(currentStatus.Logs, logEntry)

	// Keep only last 100 log entries
	if len(currentStatus.Logs) > 100 {
		currentStatus.Logs = currentStatus.Logs[len(currentStatus.Logs)-100:]
	}

	statusMutex.Unlock()
	broadcastUpdate()
}

func updateTargetStatus(zone, hostname, ip, probeType string, enabled bool) {
	statusMutex.Lock()

	// Find existing target or create new one
	found := false
	for i, target := range currentStatus.Targets {
		if target.Zone == zone && target.Hostname == hostname && target.IP == ip {
			currentStatus.Targets[i].Enabled = enabled
			currentStatus.Targets[i].ProbeType = probeType
			currentStatus.Targets[i].LastChecked = time.Now()
			found = true
			break
		}
	}

	if !found {
		newTarget := TargetStatus{
			Zone:        zone,
			Hostname:    hostname,
			IP:          ip,
			Enabled:     enabled,
			ProbeType:   probeType,
			LastChecked: time.Now(),
		}
		currentStatus.Targets = append(currentStatus.Targets, newTarget)
	}

	statusMutex.Unlock()
	broadcastUpdate()
}

