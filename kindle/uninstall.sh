#!/bin/sh
# Remove the ViaiZ KUAL extension from a Kindle mounted over USB.
# Stop the dashboard on the device first (KUAL → ViaiZ Dashboard → Stop),
# or simply reboot the Kindle: nothing is installed outside the extension dir.
#
#   ./kindle/uninstall.sh /media/$USER/Kindle
set -eu
MNT="${1:-}"
if [ -z "$MNT" ] || [ ! -d "$MNT/extensions" ]; then
	echo "usage: $0 <kindle-usb-mount-point>  (must contain extensions/)" >&2
	exit 2
fi
DEST="$MNT/extensions/viaiz-dash"
if [ ! -d "$DEST" ] || [ ! -f "$DEST/menu.json" ]; then
	echo "Nothing to remove at $DEST"
	exit 0
fi
printf "Remove %s (including dash.conf and logs)? [y/N] " "$DEST"
read -r ans
case "$ans" in
	y|Y) rm -rf "$DEST"; sync; echo "Removed." ;;
	*) echo "Aborted." ;;
esac
