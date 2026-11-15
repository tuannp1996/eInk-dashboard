#!/bin/sh
# Install the ViaiZ dashboard Scriptlet ON THE KINDLE over Wi-Fi — no USB.
# Run this from kTerm on a Sanctuary/WinterBreak jailbreak (install kTerm first
# with  ;kpm install kterm  in the homescreen search bar). It pulls the files
# from the public GitHub repo straight into place.
#
# One-liner to bootstrap it in kTerm:
#   curl -fsSL https://raw.githubusercontent.com/tuannp1996/eInk-dashboard/main/kindle/scriptlet/wifi-install.sh | sh
#   (or:  wget -qO- <same url> | sh )
set -eu

RAW="https://raw.githubusercontent.com/tuannp1996/eInk-dashboard/main"
PDIR="/mnt/us/viaiz-dash"
DOCS="/mnt/us/documents"

# Download helper: curl if present, else busybox wget.
get() { # get <url> <dest>
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	else
		wget -q -O "$2" "$1"
	fi
}

echo "ViaiZ: installing over Wi-Fi from $RAW"
mkdir -p "$PDIR/log"

get "$RAW/kindle/viaiz-dash/bin/viaiz-dash.sh"      "$PDIR/viaiz-dash.sh"
get "$RAW/kindle/viaiz-dash/dash.conf.example"      "$PDIR/dash.conf.example"
[ -f "$PDIR/dash.conf" ] || cp "$PDIR/dash.conf.example" "$PDIR/dash.conf"

get "$RAW/kindle/scriptlet/documents/viaiz-start.sh" "$DOCS/viaiz-start.sh"
get "$RAW/kindle/scriptlet/documents/viaiz-once.sh"  "$DOCS/viaiz-once.sh"
get "$RAW/kindle/scriptlet/documents/viaiz-stop.sh"  "$DOCS/viaiz-stop.sh"

for f in "$PDIR/viaiz-dash.sh" "$PDIR/dash.conf" "$PDIR/dash.conf.example" \
         "$DOCS/viaiz-start.sh" "$DOCS/viaiz-once.sh" "$DOCS/viaiz-stop.sh"; do
	tr -d '\r' < "$f" > "$f.tmp" && mv "$f.tmp" "$f"
	chmod 0755 "$f" 2>/dev/null || true
done

echo
echo "ViaiZ: installed."
echo "  1. Edit $PDIR/dash.conf  -> set SERVER_URL and DEVICE_TOKEN"
echo "     (e.g.  vi $PDIR/dash.conf  in kTerm)"
echo "  2. Go to the Library and run 'ViaiZ Dashboard - Test one frame',"
echo "     then 'ViaiZ Dashboard' to start the loop."
