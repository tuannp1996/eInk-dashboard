#!/bin/sh
# Name: ViaiZ Dashboard - Test one frame
# Author: ViaiZ
# Downloads a single dashboard PNG and draws it once. Use this to verify
# SERVER_URL / DEVICE_TOKEN / Wi-Fi before starting the loop.
DIR=/mnt/us/viaiz-dash
[ -f "$DIR/dash.conf" ] || { echo "ViaiZ: missing $DIR/dash.conf"; exit 1; }
# shellcheck disable=SC1090
. "$DIR/dash.conf"

# Make sure Wi-Fi is on (the library UI may have it off).
if [ "$(lipc-get-prop com.lab126.wifid cmState 2>/dev/null)" != "CONNECTED" ]; then
	lipc-set-prop com.lab126.cmd wirelessEnable 1 2>/dev/null
	i=0
	while [ $i -lt "${WIFI_TIMEOUT:-60}" ]; do
		[ "$(lipc-get-prop com.lab126.wifid cmState 2>/dev/null)" = "CONNECTED" ] && break
		sleep 1; i=$((i + 1))
	done
fi

OUT=/var/tmp/viaiz-once.png
URL="$SERVER_URL/api/eink/frame.png?page=${PAGE:-main}"
if command -v curl >/dev/null 2>&1; then
	curl -fsS -m 60 -H "Authorization: Bearer $DEVICE_TOKEN" -o "$OUT" "$URL"
else
	wget -q -T 60 -O "$OUT" "$URL&token=$DEVICE_TOKEN"
fi

if [ $? -eq 0 ] && [ "$(head -c 4 "$OUT" 2>/dev/null | od -An -tx1 | tr -d ' \n')" = "89504e47" ]; then
	fbink -q -c -f -W GC16 -g file="$OUT"
	echo "ViaiZ: frame shown"
else
	echo "ViaiZ: download failed — check SERVER_URL / DEVICE_TOKEN / Wi-Fi"
fi
