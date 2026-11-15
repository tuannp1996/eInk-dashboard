#!/bin/sh
# Install the ViaiZ dashboard as a Scriptlet (no KUAL) onto a Kindle mounted
# over USB. For Sanctuary / WinterBreak jailbreaks that use KPM + SH_Integration
# instead of KUAL. It only writes into <mount>/viaiz-dash and <mount>/documents —
# never firmware or system files.
#
#   ./kindle/scriptlet/install-scriptlet.sh /d           # Windows Git Bash
#   ./kindle/scriptlet/install-scriptlet.sh /media/$USER/Kindle
#   ./kindle/scriptlet/install-scriptlet.sh /Volumes/Kindle
set -eu

MNT="${1:-}"
HERE="$(cd "$(dirname "$0")" && pwd)"
KDIR="$(cd "$HERE/.." && pwd)"          # repo kindle/ directory
SRC_LOOP="$KDIR/viaiz-dash/bin/viaiz-dash.sh"
SRC_CONF="$KDIR/viaiz-dash/dash.conf.example"

if [ -z "$MNT" ] || [ ! -d "$MNT" ]; then
	echo "usage: $0 <kindle-usb-mount-point>" >&2
	exit 2
fi
if [ ! -d "$MNT/documents" ] || [ ! -d "$MNT/system" ]; then
	echo "error: $MNT does not look like a Kindle USB volume (no documents/ and system/)" >&2
	exit 1
fi
[ -f "$SRC_LOOP" ] || { echo "error: $SRC_LOOP not found" >&2; exit 1; }

PDIR="$MNT/viaiz-dash"
mkdir -p "$PDIR/log"

# Payload: the refresh loop + a config file to edit.
cp "$SRC_LOOP" "$PDIR/viaiz-dash.sh"
cp "$SRC_CONF" "$PDIR/dash.conf.example"
if [ ! -f "$PDIR/dash.conf" ]; then
	cp "$SRC_CONF" "$PDIR/dash.conf"
	echo "Created $PDIR/dash.conf — edit SERVER_URL and DEVICE_TOKEN before starting."
else
	echo "Kept existing $PDIR/dash.conf"
fi

# Scriptlet launchers -> the Library (documents/).
cp "$HERE/documents/viaiz-start.sh" "$HERE/documents/viaiz-once.sh" "$HERE/documents/viaiz-stop.sh" "$MNT/documents/"

# Kindle shell needs Unix line endings; Windows checkouts may add CR.
for f in "$PDIR/viaiz-dash.sh" "$PDIR/dash.conf" "$PDIR/dash.conf.example" \
         "$MNT/documents/viaiz-start.sh" "$MNT/documents/viaiz-once.sh" "$MNT/documents/viaiz-stop.sh"; do
	tr -d '\r' < "$f" > "$f.tmp" && mv "$f.tmp" "$f"
	chmod 0755 "$f" 2>/dev/null || true
done
sync

echo
echo "Installed (no KUAL):"
echo "  payload     -> $PDIR/"
echo "  scriptlets  -> $MNT/documents/viaiz-start.sh, viaiz-once.sh, viaiz-stop.sh"
echo
echo "Next:"
echo "  1. Edit $PDIR/dash.conf  (SERVER_URL + DEVICE_TOKEN)."
echo "  2. Eject the Kindle."
echo "  3. Library -> 'ViaiZ Dashboard - Test one frame' to verify, then"
echo "     'ViaiZ Dashboard' to start the loop. 'ViaiZ Dashboard - Stop' ends it."
