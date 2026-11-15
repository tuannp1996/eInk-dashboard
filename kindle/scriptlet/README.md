# ViaiZ Dashboard as a Scriptlet (no KUAL)

For modern Kindle jailbreaks — **Sanctuary** and **WinterBreak** — that ship
**KPM + SH_Integration** and **drop KUAL**. Instead of a KUAL extension, the
dashboard runs as **Scriptlets**: plain `.sh` files in the Kindle's `documents/`
folder that show up in the **Library** and run when you tap them.

If your jailbreak *does* have KUAL, you can use the classic extension in
[`../viaiz-dash`](../viaiz-dash) and [`../install.sh`](../install.sh) instead.
Both drive the exact same refresh loop.

## What gets installed

```
/mnt/us/viaiz-dash/            payload (not in the Library)
  viaiz-dash.sh                the refresh loop (shared with the KUAL version)
  dash.conf                    your settings — edit SERVER_URL + DEVICE_TOKEN
  dash.conf.example
  log/dash.log

/mnt/us/documents/            scriptlets (appear in the Library)
  viaiz-start.sh   -> "ViaiZ Dashboard"                 start the loop
  viaiz-once.sh    -> "ViaiZ Dashboard - Test one frame" show one frame now
  viaiz-stop.sh    -> "ViaiZ Dashboard - Stop"          stop the loop
```

Nothing is written outside `documents/` and `viaiz-dash/`. A reboot or a
**Stop** restores the screensaver; the jailbreak, firmware and system
partitions are never touched.

## Prerequisites

- A jailbroken Kindle whose hotfix provides **`fbink`** (the Sanctuary /
  WinterBreak *Universal Hotfix* installs it via `setup_fbink.sh`). Check with
  kTerm: `command -v fbink`. KUAL and MRPI are **not** needed.
- A reachable server (see the repo README) and a **device token** that matches
  one of `DEVICE_TOKENS` on the server.

## Install

### Over USB (simplest)

1. Connect the Kindle so it mounts as a drive.
2. From the repo:

   ```bash
   ./kindle/scriptlet/install-scriptlet.sh /d        # Windows Git Bash; macOS: /Volumes/Kindle
   ```

3. Edit `viaiz-dash/dash.conf` on the Kindle drive: set `SERVER_URL` and
   `DEVICE_TOKEN` (and `PAGE`, `INTERVAL` if you like).
4. **Eject** the Kindle.
5. Open the Library, run **ViaiZ Dashboard - Test one frame**. If the frame
   draws, run **ViaiZ Dashboard** to start the 15-minute loop.

### Without USB (kTerm)

If USB is unreliable, install kTerm (`;kpm install kterm`) and, with the repo
files reachable (e.g. served by your dashboard host), `curl` the three
`documents/*.sh` scriptlets into `/mnt/us/documents/`, the loop into
`/mnt/us/viaiz-dash/viaiz-dash.sh`, and create `dash.conf` there. Then run the
Library entries as above.

## Settings (`dash.conf`)

Same file and keys as the KUAL version — see
[`../../docs/KINDLE.md`](../../docs/KINDLE.md) for the full table
(`INTERVAL`, `PAGE`, `FULL_REFRESH_EVERY`, `WIFI_TIMEOUT`, `WIFI_OFF_BETWEEN`,
`SLEEP_MODE`, `STOP_FRAMEWORK`).

## Notes

- The Library title comes from the `# Name:` line inside each scriptlet, so the
  filenames stay simple (`viaiz-start.sh`) while the Library shows friendly
  names.
- Keep the Kindle on a charger for an always-on display: `SLEEP_MODE=awake`
  blocks the screensaver and gives days, not weeks, on battery.
- Logs: `/mnt/us/viaiz-dash/log/dash.log` (auto-trimmed).
