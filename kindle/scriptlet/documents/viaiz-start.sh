#!/bin/sh
# Name: ViaiZ Dashboard
# Author: ViaiZ
# Launches the dashboard refresh loop in the background. Appears in the Kindle
# Library on Sanctuary/WinterBreak jailbreaks (SH_Integration scriptlet) — no
# KUAL required. Payload lives in /mnt/us/viaiz-dash (see install-scriptlet.sh).
DIR=/mnt/us/viaiz-dash

if [ ! -f "$DIR/dash.conf" ]; then
	echo "ViaiZ: create $DIR/dash.conf from dash.conf.example and set SERVER_URL + DEVICE_TOKEN first"
	exit 1
fi

PID=/var/tmp/viaiz-dash.pid
if [ -f "$PID" ] && kill -0 "$(cat "$PID" 2>/dev/null)" 2>/dev/null; then
	echo "ViaiZ dashboard already running"
	exit 0
fi

VIAIZ_EXT_DIR="$DIR" nohup sh "$DIR/viaiz-dash.sh" >/dev/null 2>&1 &
echo "ViaiZ dashboard started — refreshing per dash.conf. Run 'ViaiZ Dashboard - Stop' to end."
