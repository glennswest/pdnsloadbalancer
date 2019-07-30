curl -v --request PATCH -H 'X-API-Key: Secret2018' http://ctl.gw.lo:8081/api/v1/servers/localhost/zones/gw.lo -d @x.json | jq .
nslookup api-int.gw.lo ctl.gw.lo

