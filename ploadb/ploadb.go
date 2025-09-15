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
 

type program struct{}

func (p *program) Start(s service.Service) error {
        ReadConfig()
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
}

var MyConfig Config

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
       comment := gjson.Get(records, "comment").String()
       probeConfig := parseProbeConfig(comment)

       for idx, host := range(recs.Array()){
           ipa := gjson.Get(host.String(),"content")
           ip := ipa.String()

           // Perform the appropriate probe based on configuration
           isHealthy := performProbe(ip, probeConfig)

           dsname := "records." + strconv.Itoa(idx) + ".disabled"
           cstate := gjson.Get(records,dsname).String()

           if isHealthy {
               if (cstate == "true"){
                   log.Printf("%s - %s changed state from disabled to enabled (%s probe)", name, ip, probeConfig.Type)
                   changed = true
               }
               records, _ = sjson.SetRaw(records,dsname,"false")
           } else {
               if (cstate == "false"){
                   log.Printf("%s - %s changed state from enabled to disabled (%s probe)", name, ip, probeConfig.Type)
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
       time.Sleep(20 * time.Second)
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

