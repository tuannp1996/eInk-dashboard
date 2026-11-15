#!/bin/sh
# Name: ViaiZ Dashboard - Stop
# Author: ViaiZ
# Stops the dashboard loop and restores the screensaver (the loop's own
# cleanup trap re-enables preventScreenSaver and restarts lab126_gui if it
# had been stopped).
PID=/var/tmp/viaiz-dash.pid
if [ -f "$PID" ] && kill "$(cat "$PID" 2>/dev/null)" 2>/dev/null; then
	echo "ViaiZ dashboard stopped"
else
	echo "ViaiZ dashboard was not running"
	# Best effort in case a previous run left the screensaver blocked.
	lipc-set-prop com.lab126.powerd preventScreenSaver 0 2>/dev/null
fi
