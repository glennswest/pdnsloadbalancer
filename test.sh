curl -v --request PATCH -H 'X-API-Key: quest.5124' http://192.168.1.51:8081/api/v1/servers/localhost/zones/gw.lo -d @x.json | jq .
nslookup api-int.gw.lo dnsx.gw.lo

