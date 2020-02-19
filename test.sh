curl -v --request PATCH -H 'X-API-Key: quest.5124' http://dns.gw.lo:8081/api/v1/servers/localhost/zones/gw.lo -d @x.json | jq .
nslookup api-int.gw.lo dns.gw.lo

