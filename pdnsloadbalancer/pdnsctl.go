package main

import "github.com/tidwall/sjson"
import "github.com/go-resty/resty"
import "fmt"

func main(){
var rrset1 = `{
            "changetype": "replace",
            "name": "zonename",
            "type": 'A',
            "ttl": 3600,
            "records": [
                {
                    "content": "something.com",
                    "name":   "192.168.1.200",
                    "disabled": "False"
                }
            ]
        }`

        domain := "gw.lo"
        hostname := "api.gw.lo"
        ipaddr   := "192.168.1.200"
        state    := "True"
        value := rrset1;
	value, _ = sjson.SetRaw(value, "name", domain)
        value, _ = sjson.SetRaw(value, "records.0.name",hostname)
        value, _ = sjson.Set(value, "records.0.content",ipaddr)
        value, _ = sjson.Set(value, "records.0.disabled",state)
       
        value, _ = sjson.SetRaw("","rrsets.0",value)
	println(value)

        // Create a Resty Client
       client := resty.New()
       resp, err := client.R().
           SetHeaders(map[string]string{
                      "Content-Type": "application/json",
                       "X-API-KEY": "Secret2018"}).
           SetBody([]byte(value)).
           Patch("http://ctl.gw.lo:8081/api/v1/servers/localhost/zones/" + domain)
	// Explore response object
	fmt.Println("Response Info:")
	fmt.Println("Error      :", err)
	fmt.Println("Status Code:", resp.StatusCode())
	fmt.Println("Status     :", resp.Status())
	fmt.Println("Time       :", resp.Time())
	fmt.Println("Received At:", resp.ReceivedAt())
	fmt.Println("Body       :\n", resp)
	fmt.Println()

}

/*
        payload = `{"rrsets": [rrset1]}`
        r = self.session.patch(
            self.url("/api/v1/servers/localhost/zones/" + name),
            data=json.dumps(payload),
            headers={'content-type': 'application/json'})
*/


