package main

import "github.com/tidwall/sjson"
import "github.com/tidwall/gjson"
import "github.com/go-resty/resty"
//import "fmt"
import "time"
import ping "github.com/sparrc/go-ping"
import "strconv"
import "log"


func getdomain(domain string) string{
      // Create a Resty Client
       client := resty.New()
       resp, _ := client.R().
           SetHeaders(map[string]string{
                      "Content-Type": "application/json",
                       "X-API-KEY": "Secret2018"}).
           Get("http://ctl.gw.lo:8081/api/v1/servers/localhost/zones/" + domain)
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


func handle_load_balance(domain string,name string,count int,records string){

       pger := []*ping.Pinger{}
       recs := gjson.Get(records,"records")
       for _,host := range(recs.Array()){
             ipa := gjson.Get(host.String(),"content")
             ip := ipa.String()
             pg, _ := ping.NewPinger(ip)
             pg.SetPrivileged(true)
             pg.Count = 3
             pger = append(pger,pg)
             go pg.Run()
             }
       time.Sleep(5 * time.Second)
       changed := false
       for idx,_ := range(recs.Array()){
           pg := pger[idx]
           stats := pg.Statistics
           //fmt.Printf("%s -> %d packs transmitted - %d packets Received \n",stats().Addr,stats().PacketsSent,stats().PacketsRecv)
           dsname := "records." + strconv.Itoa(idx) + ".disabled"
           cstate := gjson.Get(records,dsname).String()
           if (stats().PacketsRecv > 0){
              if (cstate == "true"){
                 log.Printf("%s - %s changed state to %s",name,stats().Addr,cstate)
                 changed = true
                 }
              records, _ = sjson.SetRaw(records,dsname,"false")
            } else {
              if (cstate == "false"){
                 log.Printf("%s - %s changed state to %s",name,stats().Addr,cstate)
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
       //fmt.Printf("send_update: %s\n",data)
       client := resty.New()
       resp, _ := client.R().
           SetHeaders(map[string]string{
                      "Content-Type": "application/json",
                       "X-API-KEY": "Secret2018"}).
           SetBody(data).
           Patch("http://ctl.gw.lo:8081/api/v1/servers/localhost/zones/" + domain)
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

func main(){

        for true {
            domain := "gw.lo."
            process_domain(domain)
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

