#!/bin/sh
# Install the ViaiZ KUAL extension onto a Kindle mounted over USB.
# Runs on your computer (Linux/macOS/Git Bash). It only copies files into
# <mount>/extensions/viaiz-dash — it never touches firmware or system files.
#
#   ./kindle/install.sh /media/$USER/Kindle
set -eu

MNT="${1:-}"
SRC="$(cd "$(dirname "$0")" && pwd)/viaiz-dash"

if [ -z "$MNT" ] || [ ! -d "$MNT" ]; then
	echo "usage: $0 <kindle-usb-mount-point>" >&2
	exit 2
fi
if [ ! -d "$MNT/documents" ] || [ ! -d "$MNT/system" ]; then
	echo "error: $MNT does not look like a Kindle USB volume (no documents/ and system/)" >&2
	exit 1
fi
if [ ! -d "$MNT/extensions" ]; then
	echo "error: $MNT/extensions not found — KUAL does not appear to be installed." >&2
	echo "Install KUAL + FBInk on the jailbroken Kindle first (see docs/KINDLE.md)." >&2
	exit 1
fi

DEST="$MNT/extensions/viaiz-dash"
mkdir -p "$DEST/bin" "$DEST/log"
cp "$SRC/config.xml" "$SRC/menu.json" "$SRC/dash.conf.example" "$DEST/"
cp "$SRC/bin/"*.sh "$DEST/bin/"
if [ ! -f "$DEST/dash.conf" ]; then
	cp "$SRC/dash.conf.example" "$DEST/dash.conf"
	echo "Created $DEST/dash.conf — edit SERVER_URL and DEVICE_TOKEN before starting."
else
	echo "Kept existing $DEST/dash.conf"
fi
# Make sure scripts have Unix line endings (Windows checkouts may add CR).
for f in "$DEST/bin/"*.sh "$DEST/dash.conf"; do
	tr -d '\r' < "$f" > "$f.tmp" && mv "$f.tmp" "$f"
done
sync
echo "Installed to $DEST. Eject the Kindle, open KUAL → ViaiZ Dashboard → Show one frame now."
