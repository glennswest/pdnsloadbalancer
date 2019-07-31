package main

//import "github.com/tidwall/sjson"
import "github.com/tidwall/gjson"
import "github.com/go-resty/resty"
import "fmt"
import "time"
import ping "github.com/sparrc/go-ping"


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


func handle_load_balance(name string,count int,records string){

       pger := []*ping.Pinger{}
       fmt.Printf("%s -  %d\n",name,count)
       fmt.Printf("%s\n",records)
       recs := gjson.Get(records,"records")
       for _,host := range(recs.Array()){
             ipa := gjson.Get(host.String(),"content")
             ip := ipa.String()
             fmt.Printf("ip = %s\n",ip)
             pg, _ := ping.NewPinger(ip)
             pg.SetPrivileged(true)
             pg.Count = 3
             pger = append(pger,pg)
             go pg.Run()
             }
       time.Sleep(5 * time.Second)
       for idx,_ := range(recs.Array()){
           pg := pger[idx]
           pg.Stop()
           stats := pg.Statistics
           fmt.Printf("%s -> %d packs transmitted - %d packets Received \n",stats().Addr,stats().PacketsSent,stats().PacketsRecv)
           }
       
}

func main(){


        domain := "gw.lo."
        data := getdomain(domain)
        //fmt.Printf(result)
        rrsets := gjson.Get(data,"rrsets").Array()
        //fmt.Printf(rrsets.String())
        for _,element := range rrsets{
             thename := gjson.Get(element.String(),"name").String()
             thetype := gjson.Get(element.String(),"type").String()
             entries := gjson.Get(element.String(),"records")
             cnt := len(entries.Array())
             if cnt > 1 && thetype != "" && thetype == "A"{
                handle_load_balance(thename,cnt,element.String())
                }
             }
}

