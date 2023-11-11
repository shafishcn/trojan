#!/bin/sh

privoxy --no-daemon /etc/privoxy/config &

jq ".remote_addr|=\"${REMOTE_ADDR}\"" config.json |\
jq ".remote_port|=${REMOTE_PORT}" |\
jq ".password[0]|=\"${PASSWORD}\""\
> tmpfile && cp tmpfile client.json

/usr/local/bin/trojan --config=/config/client.json
