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
import "github.com/coreos/go-systemd/v22/daemon"
import "github.com/BurntSushi/toml"
import "net/http"
import "crypto/tls"
import "encoding/json"
import "strings"
import "net"
import "sync"
import "github.com/gorilla/websocket"
import "github.com/miekg/dns"
import "os/exec"
import "context"
 

type program struct{}

func (p *program) Start(s service.Service) error {
        ReadConfig()
	startWebServer()
	// Clear DNS caches on startup for a clean state
	clearAllDNSCaches()
	// Notify systemd that we're ready
	daemon.SdNotify(false, daemon.SdNotifyReady)
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
	Hostname        string    `json:"hostname"`
	IP              string    `json:"ip"`
	Enabled         bool      `json:"enabled"`
	ProbeType       string    `json:"probeType"`
	LastChecked     time.Time `json:"lastChecked"`
	Zone            string    `json:"zone"`
	StateChangedAt  time.Time `json:"stateChangedAt"`
	CurrentUptime   string    `json:"currentUptime"`
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

type DNSServerResult struct {
	ResolvedIPs []string  `json:"resolvedIPs"`
	TTL         int       `json:"ttl"`
	Server      string    `json:"server"`
	LastQueried time.Time `json:"lastQueried"`
}

type DNSResolution struct {
	Hostname    string            `json:"hostname"`
	Zone        string            `json:"zone"`
	Primary     DNSServerResult   `json:"primary"`     // PowerDNS Authoritative (port 54)
	Recursor    DNSServerResult   `json:"recursor"`    // PowerDNS Recursor (port 53)
	LastQueried time.Time         `json:"lastQueried"`
}

type StatusData struct {
	Targets     []TargetStatus  `json:"targets"`
	Logs        []LogEntry      `json:"logs"`
	Resolutions []DNSResolution `json:"resolutions"`
	Uptime      string          `json:"uptime"`
}

// Global state for web GUI
var (
	statusMutex   sync.RWMutex
	serviceStartTime time.Time = time.Now()
	currentStatus StatusData = StatusData{
		Targets:     make([]TargetStatus, 0),
		Logs:        make([]LogEntry, 0),
		Resolutions: make([]DNSResolution, 0),
	}
	wsClients     = make(map[*websocket.Conn]bool)
	wsClientsMutex sync.RWMutex
	upgrader      = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for simplicity
		},
	}
	// Goroutine limiting
	maxConcurrentChecks = 10
	checkSemaphore     = make(chan struct{}, maxConcurrentChecks)
	// Record-level locking to prevent race conditions
	recordLocks = make(map[string]*sync.Mutex)
	recordLocksMutex sync.RWMutex
)

type ProbeConfig struct {
	Type     string `json:"type"`     // "ping", "http", "https", or "tcp"
	Path     string `json:"path"`     // HTTP path (optional, default "/")
	Port     int    `json:"port"`     // Port (optional, defaults: 80 for http, 443 for https, required for tcp)
	Timeout  int    `json:"timeout"`  // Timeout in seconds (optional, default 5)
	Expected int    `json:"expected"` // Expected HTTP status code (optional, default 200)
}

// Format uptime duration using the largest appropriate unit
func formatUptime(duration time.Duration) string {
	totalSeconds := int64(duration.Seconds())

	// Define time units in seconds
	const (
		secondsPerMinute = 60
		secondsPerHour   = 3600
		secondsPerDay    = 86400
		secondsPerMonth  = 2592000  // 30 days
		secondsPerYear   = 31536000 // 365 days
	)

	// Check from largest to smallest unit
	if totalSeconds >= secondsPerYear {
		years := totalSeconds / secondsPerYear
		if years == 1 {
			return "1 yr"
		}
		return fmt.Sprintf("%d yr", years)
	}

	if totalSeconds >= secondsPerMonth {
		months := totalSeconds / secondsPerMonth
		if months == 1 {
			return "1 mo"
		}
		return fmt.Sprintf("%d mo", months)
	}

	if totalSeconds >= secondsPerDay {
		days := totalSeconds / secondsPerDay
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}

	if totalSeconds >= secondsPerHour {
		hours := totalSeconds / secondsPerHour
		if hours == 1 {
			return "1 hr"
		}
		return fmt.Sprintf("%d hr", hours)
	}

	if totalSeconds >= secondsPerMinute {
		minutes := totalSeconds / secondsPerMinute
		if minutes == 1 {
			return "1 min"
		}
		return fmt.Sprintf("%d min", minutes)
	}

	// Default to seconds
	if totalSeconds == 1 {
		return "1 sec"
	}
	return fmt.Sprintf("%d sec", totalSeconds)
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

func queryDNSServer(hostname, server string) ([]string, int, error) {
	// Remove trailing dot if present
	hostname = strings.TrimSuffix(hostname, ".")

	c := dns.Client{Timeout: 5 * time.Second}
	m := dns.Msg{}
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)

	r, _, err := c.Exchange(&m, server)
	if err != nil {
		return nil, 0, err
	}

	// Handle different DNS response codes
	switch r.Rcode {
	case dns.RcodeSuccess:
		// Success - parse A records
		var ips []string
		var ttl int = 300 // default TTL

		for _, ans := range r.Answer {
			if a, ok := ans.(*dns.A); ok {
				ips = append(ips, a.A.String())
				ttl = int(a.Hdr.Ttl)
			}
		}

		// Return empty list if no A records found (valid response)
		return ips, ttl, nil

	case dns.RcodeNameError: // NXDOMAIN
		// Domain doesn't exist - return empty list (not an error for GUI purposes)
		return []string{}, 0, nil

	default:
		// Other DNS errors (SERVFAIL, REFUSED, etc.)
		return nil, 0, fmt.Errorf("DNS error code: %d", r.Rcode)
	}
}

func resolveDNSWithTTL(hostname string) ([]string, int, error) {
	// Remove trailing dot if present
	hostname = strings.TrimSuffix(hostname, ".")

	c := dns.Client{Timeout: 5 * time.Second}
	m := dns.Msg{}
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)

	// Try local PowerDNS Authoritative server first (port 54), then recursor (port 53), then public DNS servers
	var dnsServers []string
	if strings.HasPrefix(MyConfig.Baseurl, "http://") {
		// Extract hostname from PowerDNS API URL
		apiURL := strings.TrimPrefix(MyConfig.Baseurl, "http://")
		if colonIndex := strings.Index(apiURL, ":"); colonIndex != -1 {
			dnsHost := apiURL[:colonIndex]
			// Try authoritative server first (port 54), then recursor (port 53)
			dnsServers = append(dnsServers, dnsHost+":54", dnsHost+":53")
		}
	}
	// Add public DNS servers as fallback
	dnsServers = append(dnsServers, "8.8.8.8:53", "1.1.1.1:53", "208.67.222.222:53")

	for _, server := range dnsServers {
		r, _, err := c.Exchange(&m, server)
		if err != nil {
			continue
		}

		if r.Rcode != dns.RcodeSuccess {
			continue
		}

		var ips []string
		var ttl int = 300 // default TTL

		for _, ans := range r.Answer {
			if a, ok := ans.(*dns.A); ok {
				ips = append(ips, a.A.String())
				ttl = int(a.Hdr.Ttl)
			}
		}

		if len(ips) > 0 {
			return ips, ttl, nil
		}
	}

	return nil, 0, fmt.Errorf("failed to resolve %s", hostname)
}

func clearRecursorCache(hostname string) error {
	// Use rec_control to clear cache for specific hostname
	cmd := fmt.Sprintf("rec_control wipe-cache %s", hostname)

	// Execute the command with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	execCmd := exec.CommandContext(ctx, "sh", "-c", cmd)
	output, err := execCmd.Output()
	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout clearing recursor cache for %s", hostname)
		}
		// Check if it's a command not found error
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("rec_control command failed for %s: exit code %d, stderr: %s",
				hostname, exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to clear recursor cache for %s: %v", hostname, err)
	}

	outputStr := strings.TrimSpace(string(output))
	log.Printf("Cleared recursor cache for %s: %s", hostname, outputStr)

	// Check if the output indicates success (rec_control typically reports what was wiped)
	if !strings.Contains(outputStr, "wiped") && outputStr != "" {
		log.Printf("Warning: Unexpected output from rec_control for %s: %s", hostname, outputStr)
	}

	return nil
}

func clearRecursorZoneCache(zone string) error {
	// Clear cache for entire zone
	cmd := fmt.Sprintf("rec_control wipe-cache %s", zone)

	// Execute the command with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	execCmd := exec.CommandContext(ctx, "sh", "-c", cmd)
	output, err := execCmd.Output()
	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout clearing recursor cache for zone %s", zone)
		}
		// Check if it's a command not found error
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("rec_control command failed for zone %s: exit code %d, stderr: %s",
				zone, exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to clear recursor cache for zone %s: %v", zone, err)
	}

	outputStr := strings.TrimSpace(string(output))
	log.Printf("Cleared recursor cache for zone %s: %s", zone, outputStr)

	// Check if the output indicates success
	if !strings.Contains(outputStr, "wiped") && outputStr != "" {
		log.Printf("Warning: Unexpected output from rec_control for zone %s: %s", zone, outputStr)
	}

	return nil
}

func clearAuthoritativeCache() error {
	// Clear PowerDNS Authoritative cache using pdns_control
	cmd := "pdns_control purge"

	// Execute the command with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	execCmd := exec.CommandContext(ctx, "sh", "-c", cmd)
	output, err := execCmd.Output()
	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout clearing authoritative cache")
		}
		// Check if it's a command not found error
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Some versions don't have cache or purge command, that's okay
			log.Printf("Note: pdns_control purge not available or cache disabled: %s", string(exitErr.Stderr))
			return nil
		}
		return fmt.Errorf("failed to clear authoritative cache: %v", err)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr != "" {
		log.Printf("Cleared authoritative cache: %s", outputStr)
	}

	return nil
}

func clearRecursorFullCache() error {
	// Clear entire PowerDNS Recursor cache
	cmd := "rec_control wipe-cache ."

	// Execute the command with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	execCmd := exec.CommandContext(ctx, "sh", "-c", cmd)
	output, err := execCmd.Output()
	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout clearing full recursor cache")
		}
		// Check if it's a command not found error
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("rec_control command failed: exit code %d, stderr: %s",
				exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to clear full recursor cache: %v", err)
	}

	outputStr := strings.TrimSpace(string(output))
	log.Printf("Cleared full recursor cache: %s", outputStr)

	return nil
}

func clearAllDNSCaches() {
	log.Printf("Clearing all DNS caches on startup...")

	// Clear PowerDNS Authoritative cache (if available)
	if err := clearAuthoritativeCache(); err != nil {
		log.Printf("Warning: Could not clear authoritative cache: %v", err)
	}

	// Clear PowerDNS Recursor cache
	if err := clearRecursorFullCache(); err != nil {
		log.Printf("Warning: Could not clear recursor cache: %v", err)
	}

	log.Printf("DNS cache clearing completed")
}

// Check for and clear stale discrete entries that might conflict with wildcard resolution
func checkAndClearStaleDiscreteEntries(domain string, wildcardZone string) {
	log.Printf("Clearing discrete entries from recursor cache for wildcard zone %s", wildcardZone)

	// Get all A records from the zone to identify discrete entries
	data := getdomain(domain)
	rrsets := gjson.Get(data, "rrsets").Array()

	// Track discrete (non-wildcard) hostnames in this zone
	var discreteEntries []string
	for _, element := range rrsets {
		thename := gjson.Get(element.String(), "name").String()
		thetype := gjson.Get(element.String(), "type").String()

		// Look for A records that are not wildcards but are in the wildcard zone
		if thetype == "A" && !strings.HasPrefix(thename, "*.") && strings.HasSuffix(thename, wildcardZone) {
			// This is a discrete entry that might have stale cache
			discreteEntries = append(discreteEntries, thename)
		}
	}

	// Clear recursor cache for each discrete entry found - ALWAYS clear them on state change
	if len(discreteEntries) > 0 {
		log.Printf("Found %d discrete entries in wildcard zone %s, clearing all from recursor cache", len(discreteEntries), wildcardZone)
		for _, entry := range discreteEntries {
			// Always clear cache for discrete entries when wildcard state changes
			log.Printf("Clearing recursor cache for discrete entry: %s", entry)
			if err := clearRecursorCache(entry); err != nil {
				log.Printf("Warning: Failed to clear cache for %s: %v", entry, err)
			}
		}
	}

	// Also clear cache for common OpenShift/Kubernetes subdomains that might exist
	// These are cleared regardless of whether they exist in PowerDNS to ensure fresh resolution
	commonSubdomains := []string{
		"oauth-openshift",
		"console-openshift-console",
		"oauth-openshift-console",
		"downloads-openshift-console",
		"grafana",
		"prometheus",
		"alertmanager",
		"oauth2-proxy",
		"ingress",
		"router",
		"registry",
		"image-registry",
		"metrics",
		"monitoring",
		"logging",
	}

	clearedCount := 0
	for _, subdomain := range commonSubdomains {
		hostname := subdomain + "." + wildcardZone

		// Always try to clear cache for these entries
		cmd := fmt.Sprintf("rec_control wipe-cache '%s'", hostname)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		execCmd := exec.CommandContext(ctx, "sh", "-c", cmd)
		output, err := execCmd.Output()
		cancel()

		if err == nil {
			outputStr := strings.TrimSpace(string(output))
			// Check if any entries were cleared (negative or positive)
			if strings.Contains(outputStr, "wiped") {
				// Parse the output to see if anything was actually cleared
				if !strings.Contains(outputStr, "wiped 0 records, 0 negative") {
					log.Printf("Cleared cache for %s: %s", hostname, outputStr)
					clearedCount++
				}
			}
		}
	}

	if clearedCount > 0 {
		log.Printf("Cleared %d common subdomain entries from recursor cache for zone %s", clearedCount, wildcardZone)
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

// Get or create a mutex for this specific record
func getRecordMutex(recordKey string) *sync.Mutex {
	recordLocksMutex.Lock()
	defer recordLocksMutex.Unlock()

	if recordLocks[recordKey] == nil {
		recordLocks[recordKey] = &sync.Mutex{}
	}
	return recordLocks[recordKey]
}

func handle_load_balance(domain string,name string,count int,records string){
	// Acquire semaphore to limit concurrent health checks
	checkSemaphore <- struct{}{}
	defer func() { <-checkSemaphore }()

	// Use record-specific locking to prevent race conditions
	recordKey := domain + ":" + name
	recordMutex := getRecordMutex(recordKey)
	recordMutex.Lock()
	defer recordMutex.Unlock()

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

           // Check if state is changing
           stateChanged := false
           if isHealthy {
               if (cstate == "true"){
                   log.Printf("%s - %s changed state from disabled to enabled (%s probe)", name, ip, probeConfig.Type)
                   addLogEntry(name, ip, "disabled", "enabled", probeConfig.Type)
                   stateChanged = true
                   changed = true
               }
               records, _ = sjson.SetRaw(records,dsname,"false")
           } else {
               if (cstate == "false"){
                   log.Printf("%s - %s changed state from enabled to disabled (%s probe)", name, ip, probeConfig.Type)
                   addLogEntry(name, ip, "enabled", "disabled", probeConfig.Type)
                   stateChanged = true
                   changed = true
               }
               records, _ = sjson.SetRaw(records,dsname,"true")
           }

           // Update target status for web GUI with state change information
           updateTargetStatus(domain, name, ip, probeConfig.Type, isHealthy, stateChanged)
       }

       // Failsafe: if all hosts are disabled, enable the first one
       // This ensures DNS queries always return at least one IP
       allDisabled := true
       for idx := range recs.Array() {
           dsname := "records." + strconv.Itoa(idx) + ".disabled"
           if gjson.Get(records, dsname).String() == "false" {
               allDisabled = false
               break
           }
       }

       if allDisabled && len(recs.Array()) > 0 {
           firstIP := gjson.Get(recs.Array()[0].String(), "content").String()
           log.Printf("%s - all hosts unavailable, enabling first entry %s as failsafe", name, firstIP)
           addLogEntry(name, firstIP, "disabled", "enabled (failsafe)", probeConfig.Type)
           records, _ = sjson.SetRaw(records, "records.0.disabled", "false")
           changed = true
           // Update target status for the failsafe-enabled host
           updateTargetStatus(domain, name, firstIP, probeConfig.Type, true, true)
       }

       if (changed == true){
           send_update(domain,name,records)
       }

       // Update DNS resolution status for this hostname
       go updateDNSResolution(domain, name)
}

func send_update(domain string,name string,records string) string{
// Create a Resty Client
       data, _  := sjson.SetRaw("","rrsets.0",records)
       data, _ = sjson.Set(data,"rrsets.0.changetype", "replace")
       // Set TTL to 5 seconds for load balanced records
       data, _ = sjson.Set(data,"rrsets.0.ttl", 5)
       fmt.Printf("send_update: %s\n",data)
       client := resty.New()
       client.SetHostURL(MyConfig.Baseurl)
       resp, _ := client.R().
           SetHeaders(map[string]string{
                      "Content-Type": "application/json",
                       "X-API-KEY": MyConfig.ApiPassword}).
           SetBody(data).
           Patch("/api/v1/servers/localhost/zones/" + domain)

        // If the update was successful, clear the recursor cache
        if resp.StatusCode() >= 200 && resp.StatusCode() < 300 {
            // Clear cache for the specific hostname that was updated
            go func() {
                if err := clearRecursorCache(name); err != nil {
                    log.Printf("Warning: Failed to clear recursor cache for %s: %v", name, err)
                }

                // For wildcard domains, also clear negative cache entries that might affect wildcard resolution
                if strings.HasPrefix(name, "*.") {
                    // Clear the wildcard zone cache to ensure negative entries don't interfere
                    zoneName := strings.TrimPrefix(name, "*.")
                    if err := clearRecursorZoneCache(zoneName); err != nil {
                        log.Printf("Warning: Failed to clear recursor zone cache for wildcard %s: %v", zoneName, err)
                    }
                    log.Printf("Cleared recursor cache for wildcard zone: %s", zoneName)

                    // Also check and clear any discrete stale entries that might conflict with wildcard
                    go checkAndClearStaleDiscreteEntries(domain, zoneName)
                }
            }()
        } else {
            log.Printf("DNS update failed for %s, status: %d, not clearing cache", name, resp.StatusCode())
        }

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
        // Notify systemd watchdog that we're alive
        daemon.SdNotify(false, daemon.SdNotifyWatchdog)

        // Track active targets in this cycle to clean up stale ones
        activeTargets := make(map[string]bool)

        domainsjs := getdomainlist()
        domains := gjson.Parse(domainsjs).Array()
        for _,domain := range domains{
              process_domain(domain.String(), activeTargets)
              }

        // Clean up targets that are no longer active
        cleanupStaleTargets(activeTargets)

       time.Sleep(10 * time.Second)
        }
}

func process_domain(domain string, activeTargets map[string]bool){


        data := getdomain(domain)
        rrsets := gjson.Get(data,"rrsets").Array()
        for _,element := range rrsets{
             thename := gjson.Get(element.String(),"name").String()
             thetype := gjson.Get(element.String(),"type").String()
             entries := gjson.Get(element.String(),"records")
             cnt := len(entries.Array())
             if cnt > 1 && thetype != "" && thetype == "A"{
                // Mark all IPs in this record as active
                for _, host := range entries.Array() {
                    ip := gjson.Get(host.String(), "content").String()
                    targetKey := domain + ":" + thename + ":" + ip
                    activeTargets[targetKey] = true
                }
                go handle_load_balance(domain,thename,cnt,element.String())
                }
             }
}

// Clean up targets that are no longer present in PowerDNS
func cleanupStaleTargets(activeTargets map[string]bool) {
	statusMutex.Lock()
	defer statusMutex.Unlock()

	var updatedTargets []TargetStatus
	var removedCount int
	var removedTargets []TargetStatus

	for _, target := range currentStatus.Targets {
		targetKey := target.Zone + ":" + target.Hostname + ":" + target.IP
		if activeTargets[targetKey] {
			// Target is still active, keep it
			updatedTargets = append(updatedTargets, target)
		} else {
			// Target is stale, remove it
			log.Printf("Removing stale target: %s - %s (zone: %s)", target.Hostname, target.IP, target.Zone)
			removedTargets = append(removedTargets, target)
			removedCount++
		}
	}

	if removedCount > 0 {
		currentStatus.Targets = updatedTargets
		log.Printf("Cleaned up %d stale targets", removedCount)

		// Add removal entries to the GUI log
		for _, removed := range removedTargets {
			logEntry := LogEntry{
				Timestamp: time.Now(),
				Hostname:  removed.Hostname,
				IP:        removed.IP,
				OldState:  "monitored",
				NewState:  "removed",
				ProbeType: removed.ProbeType,
				Message:   fmt.Sprintf("%s - %s removed from monitoring (record deleted from DNS)", removed.Hostname, removed.IP),
			}
			currentStatus.Logs = append(currentStatus.Logs, logEntry)
		}

		// Keep only last 100 log entries
		if len(currentStatus.Logs) > 100 {
			currentStatus.Logs = currentStatus.Logs[len(currentStatus.Logs)-100:]
		}

		// Broadcast update outside of mutex lock
		go broadcastUpdate()
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
            padding: 10px;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1600px;
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
            grid-template-columns: 1fr 1fr 1fr;
            gap: 15px;
            padding: 15px;
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
            max-height: 600px;
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
        .log-entry.removed-entry {
            background: #f8d7da;
            border-left: 3px solid #dc3545;
        }
        .log-entry.enabled-entry {
            background: #d4edda;
            border-left: 3px solid #28a745;
        }
        .log-entry.disabled-entry {
            background: #f8d7da;
            border-left: 3px solid #ffc107;
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
        .dns-resolution {
            font-family: monospace;
        }
        .resolved-ip {
            margin-left: 20px;
            padding: 2px 6px;
            margin: 2px 0;
            background: #e9ecef;
            border-radius: 3px;
            font-size: 0.9em;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .ttl-badge {
            font-size: 0.7em;
            padding: 1px 4px;
            background: #17a2b8;
            color: white;
            border-radius: 8px;
        }
        .dns-server-section {
            margin-left: 20px;
            margin-bottom: 10px;
            border-left: 2px solid #dee2e6;
            padding-left: 10px;
        }
        .dns-server-title {
            font-weight: bold;
            font-size: 0.9em;
            margin-bottom: 5px;
            color: #495057;
        }
        .primary-server {
            border-left-color: #28a745;
        }
        .recursor-server {
            border-left-color: #007bff;
        }
        .server-badge {
            font-size: 0.6em;
            padding: 1px 3px;
            color: white;
            border-radius: 6px;
            margin-left: 5px;
        }
        .primary-badge {
            background: #28a745;
        }
        .recursor-badge {
            background: #007bff;
        }
        @media (max-width: 1024px) {
            .dashboard {
                grid-template-columns: 1fr 1fr;
            }
            .panel-content {
                max-height: 400px;
            }
        }
        @media (max-width: 768px) {
            .dashboard {
                grid-template-columns: 1fr;
            }
            .panel-content {
                max-height: 350px;
            }
            body {
                padding: 5px;
            }
        }
        @media (min-width: 1400px) {
            .panel-content {
                max-height: 700px;
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
                <div class="panel-header">Current DNS Resolution <span id="uptimeDisplay" style="float: right; font-weight: normal; font-size: 0.9em;">Uptime: loading...</span></div>
                <div class="panel-content">
                    <div id="dnsTree" class="dns-resolution">
                        <div>Loading DNS resolutions...</div>
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
                updateDNSResolutions(data.resolutions);
                updateLogs(data.logs);
                updateUptime(data.uptime);
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
            if (!targetTree) return;

            // Ensure targets is an array
            if (!targets || !Array.isArray(targets)) {
                targetTree.innerHTML = '<div>No load balanced targets found</div>';
                return;
            }

            const zones = {};

            // Group targets by zone
            targets.forEach(target => {
                if (!target) return;
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
                        const uptime = target.currentUptime || 'N/A';
                        const arrow = target.enabled ? '↑' : '↓';
                        const arrowColor = target.enabled ? '#00AA00' : '#FF0000';
                        html += '<div class="target ' + statusClass + '">';
                        html += '<span>📍 ' + target.ip + ' <span class="probe-type">' + target.probeType + '</span></span>';
                        html += '<span>';
                        html += '<span class="status-badge status-' + statusClass + '">' + statusBadge + '</span>';
                        html += ' <small style="color: #666;">(' + uptime + ' <span style="color: ' + arrowColor + '; font-weight: 900; font-size: 1.6em; text-shadow: 1px 1px 2px rgba(0,0,0,0.5);">' + arrow + '</span>)</small>';
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

        function updateDNSResolutions(resolutions) {
            const dnsTree = document.getElementById('dnsTree');
            if (!dnsTree) return;

            // Ensure resolutions is an array
            if (!resolutions || !Array.isArray(resolutions)) {
                dnsTree.innerHTML = '<div>No DNS resolutions available</div>';
                return;
            }

            const zones = {};

            // Group resolutions by zone
            resolutions.forEach(resolution => {
                if (!resolution) return;
                if (!zones[resolution.zone]) {
                    zones[resolution.zone] = [];
                }
                zones[resolution.zone].push(resolution);
            });

            let html = '';
            Object.keys(zones).sort().forEach(zone => {
                html += '<div class="zone">📁 ' + zone + '</div>';
                zones[zone].sort((a, b) => a.hostname.localeCompare(b.hostname)).forEach(resolution => {
                    html += '<div class="hostname">🔍 ' + resolution.hostname + '</div>';

                    // Primary (Authoritative) Server
                    if (resolution.primary && resolution.primary.resolvedIPs && resolution.primary.resolvedIPs.length > 0) {
                        html += '<div class="dns-server-section primary-server">';
                        html += '<div class="dns-server-title">Primary (Authoritative)<span class="server-badge primary-badge">:54</span></div>';
                        resolution.primary.resolvedIPs.forEach(ip => {
                            html += '<div class="resolved-ip">';
                            html += '<span>🌐 ' + ip + '</span>';
                            html += '<span class="ttl-badge">TTL: ' + resolution.primary.ttl + 's</span>';
                            html += '</div>';
                        });
                        const primaryQueried = new Date(resolution.primary.lastQueried).toLocaleTimeString();
                        html += '<div style="font-size: 0.7em; color: #6c757d;">Last queried: ' + primaryQueried + '</div>';
                        html += '</div>';
                    }

                    // Recursor Server
                    if (resolution.recursor && resolution.recursor.resolvedIPs && resolution.recursor.resolvedIPs.length > 0) {
                        html += '<div class="dns-server-section recursor-server">';
                        html += '<div class="dns-server-title">Recursor (Client View)<span class="server-badge recursor-badge">:53</span></div>';
                        resolution.recursor.resolvedIPs.forEach(ip => {
                            html += '<div class="resolved-ip">';
                            html += '<span>🌐 ' + ip + '</span>';
                            html += '<span class="ttl-badge">TTL: ' + resolution.recursor.ttl + 's</span>';
                            html += '</div>';
                        });
                        const recursorQueried = new Date(resolution.recursor.lastQueried).toLocaleTimeString();
                        html += '<div style="font-size: 0.7em; color: #6c757d;">Last queried: ' + recursorQueried + '</div>';
                        html += '</div>';
                    }
                });
            });

            if (html === '') {
                html = '<div>No DNS resolutions available</div>';
            }

            dnsTree.innerHTML = html;
        }

        function updateLogs(logs) {
            const logBody = document.getElementById('logBody');
            if (!logBody) return;

            let html = '';

            // Ensure logs is an array before processing
            if (!logs || !Array.isArray(logs)) {
                html = '<tr><td colspan="5">No status changes logged yet</td></tr>';
                logBody.innerHTML = html;
                return;
            }

            // Process the last 50 logs, newest first
            const logsToShow = logs.slice(-50).reverse();

            logsToShow.forEach(log => {
                if (!log) return; // Skip null/undefined entries

                const timestamp = new Date(log.timestamp).toLocaleString();
                const changeText = log.oldState + ' → ' + log.newState;

                // Determine row class based on state change
                let rowClass = 'log-entry state-change';
                if (log.newState === 'removed') {
                    rowClass += ' removed-entry';
                } else if (log.newState === 'enabled') {
                    rowClass += ' enabled-entry';
                } else if (log.newState === 'disabled') {
                    rowClass += ' disabled-entry';
                }

                html += '<tr class="' + rowClass + '">';
                html += '<td class="timestamp">' + timestamp + '</td>';
                html += '<td>' + (log.hostname || '') + '</td>';
                html += '<td>' + (log.ip || '') + '</td>';
                html += '<td>' + changeText + '</td>';
                html += '<td><span class="probe-type">' + (log.probeType || '') + '</span></td>';
                html += '</tr>';
            });

            if (html === '') {
                html = '<tr><td colspan="5">No status changes logged yet</td></tr>';
            }

            logBody.innerHTML = html;
        }

        function updateUptime(uptime) {
            const uptimeDisplay = document.getElementById('uptimeDisplay');
            if (uptimeDisplay && uptime) {
                uptimeDisplay.textContent = 'Uptime: ' + uptime;
            }
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
	currentStatus.Uptime = formatUptime(time.Since(serviceStartTime))

	// Update all target uptimes
	now := time.Now()
	for i := range currentStatus.Targets {
		if !currentStatus.Targets[i].StateChangedAt.IsZero() {
			uptime := now.Sub(currentStatus.Targets[i].StateChangedAt)
			currentStatus.Targets[i].CurrentUptime = formatUptime(uptime)
		}
	}

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
	// Update service uptime
	currentStatus.Uptime = formatUptime(time.Since(serviceStartTime))

	// Update all target uptimes before marshaling
	now := time.Now()
	for i := range currentStatus.Targets {
		if !currentStatus.Targets[i].StateChangedAt.IsZero() {
			uptime := now.Sub(currentStatus.Targets[i].StateChangedAt)
			currentStatus.Targets[i].CurrentUptime = formatUptime(uptime)
		}
	}

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
	logEntry := LogEntry{
		Timestamp: time.Now(),
		Hostname:  hostname,
		IP:        ip,
		OldState:  oldState,
		NewState:  newState,
		ProbeType: probeType,
		Message:   fmt.Sprintf("%s - %s changed state from %s to %s (%s probe)", hostname, ip, oldState, newState, probeType),
	}

	statusMutex.Lock()
	currentStatus.Logs = append(currentStatus.Logs, logEntry)

	// Keep only last 100 log entries
	if len(currentStatus.Logs) > 100 {
		currentStatus.Logs = currentStatus.Logs[len(currentStatus.Logs)-100:]
	}
	statusMutex.Unlock()

	// Broadcast update outside of mutex lock to prevent deadlock
	broadcastUpdate()
}

func updateTargetStatus(zone, hostname, ip, probeType string, enabled bool, stateChanged bool) {
	statusMutex.Lock()
	now := time.Now()

	// Find existing target or create new one
	found := false
	for i, target := range currentStatus.Targets {
		if target.Zone == zone && target.Hostname == hostname && target.IP == ip {
			// Update basic info
			currentStatus.Targets[i].Enabled = enabled
			currentStatus.Targets[i].ProbeType = probeType
			currentStatus.Targets[i].LastChecked = now

			// If state changed, update the state change timestamp
			if stateChanged {
				currentStatus.Targets[i].StateChangedAt = now
			}

			// Calculate uptime in current state
			if !currentStatus.Targets[i].StateChangedAt.IsZero() {
				uptime := time.Since(currentStatus.Targets[i].StateChangedAt)
				currentStatus.Targets[i].CurrentUptime = formatUptime(uptime)
			} else {
				// If no state change timestamp, set it to now (for existing records)
				currentStatus.Targets[i].StateChangedAt = now
				currentStatus.Targets[i].CurrentUptime = "1 sec"
			}

			found = true
			break
		}
	}

	if !found {
		newTarget := TargetStatus{
			Zone:           zone,
			Hostname:       hostname,
			IP:             ip,
			Enabled:        enabled,
			ProbeType:      probeType,
			LastChecked:    now,
			StateChangedAt: now,
			CurrentUptime:  "1 sec",
		}
		currentStatus.Targets = append(currentStatus.Targets, newTarget)
	}
	statusMutex.Unlock()

	// Broadcast update outside of mutex lock to prevent deadlock
	broadcastUpdate()
}

func updateDNSResolution(zone, hostname string) {
	now := time.Now()

	// Get DNS host from config
	var dnsHost string
	if strings.HasPrefix(MyConfig.Baseurl, "http://") {
		apiURL := strings.TrimPrefix(MyConfig.Baseurl, "http://")
		if colonIndex := strings.Index(apiURL, ":"); colonIndex != -1 {
			dnsHost = apiURL[:colonIndex]
		}
	}

	if dnsHost == "" {
		log.Printf("Could not extract DNS host from config for %s", hostname)
		return
	}

	// Query both primary (authoritative) and recursor
	primaryServer := dnsHost + ":54"
	recursorServer := dnsHost + ":53"

	primaryIPs, primaryTTL, primaryErr := queryDNSServer(hostname, primaryServer)
	recursorIPs, recursorTTL, recursorErr := queryDNSServer(hostname, recursorServer)

	statusMutex.Lock()

	// Find existing resolution or create new one
	found := false
	for i, resolution := range currentStatus.Resolutions {
		if resolution.Zone == zone && resolution.Hostname == hostname {
			// Always update primary data (empty list is valid for disabled records)
			if primaryErr == nil {
				currentStatus.Resolutions[i].Primary = DNSServerResult{
					ResolvedIPs: primaryIPs,
					TTL:         primaryTTL,
					Server:      primaryServer,
					LastQueried: now,
				}
			} else {
				// Clear primary data on error
				currentStatus.Resolutions[i].Primary = DNSServerResult{
					ResolvedIPs: []string{},
					TTL:         0,
					Server:      primaryServer,
					LastQueried: now,
				}
			}

			// Always update recursor data (empty list is valid for disabled records)
			if recursorErr == nil {
				currentStatus.Resolutions[i].Recursor = DNSServerResult{
					ResolvedIPs: recursorIPs,
					TTL:         recursorTTL,
					Server:      recursorServer,
					LastQueried: now,
				}
			} else {
				// Clear recursor data on error
				currentStatus.Resolutions[i].Recursor = DNSServerResult{
					ResolvedIPs: []string{},
					TTL:         0,
					Server:      recursorServer,
					LastQueried: now,
				}
			}

			currentStatus.Resolutions[i].LastQueried = now
			found = true
			break
		}
	}

	if !found {
		newResolution := DNSResolution{
			Zone:        zone,
			Hostname:    hostname,
			LastQueried: now,
		}

		// Always set primary data (empty list is valid for disabled records)
		if primaryErr == nil {
			newResolution.Primary = DNSServerResult{
				ResolvedIPs: primaryIPs,
				TTL:         primaryTTL,
				Server:      primaryServer,
				LastQueried: now,
			}
		} else {
			newResolution.Primary = DNSServerResult{
				ResolvedIPs: []string{},
				TTL:         0,
				Server:      primaryServer,
				LastQueried: now,
			}
		}

		// Always set recursor data (empty list is valid for disabled records)
		if recursorErr == nil {
			newResolution.Recursor = DNSServerResult{
				ResolvedIPs: recursorIPs,
				TTL:         recursorTTL,
				Server:      recursorServer,
				LastQueried: now,
			}
		} else {
			newResolution.Recursor = DNSServerResult{
				ResolvedIPs: []string{},
				TTL:         0,
				Server:      recursorServer,
				LastQueried: now,
			}
		}

		currentStatus.Resolutions = append(currentStatus.Resolutions, newResolution)
	}

	statusMutex.Unlock()

	// Log any errors
	if primaryErr != nil {
		log.Printf("Primary DNS resolution failed for %s: %v", hostname, primaryErr)
	}
	if recursorErr != nil {
		log.Printf("Recursor DNS resolution failed for %s: %v", hostname, recursorErr)
	}

	// Broadcast update outside of mutex lock to prevent deadlock
	broadcastUpdate()
}

