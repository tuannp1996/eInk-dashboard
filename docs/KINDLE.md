# Kindle Paperwhite 3 setup

The PW3 (7th generation) has a 1072×1448 panel at 300 ppi with 16 gray levels. The server renders exactly that size.

This project **does not jailbreak, flash or modify** your Kindle. Mode A needs nothing extra. Mode B needs a Kindle that you have already jailbroken yourself, with KUAL and FBInk installed. Whether a jailbreak exists depends on your firmware version, and this project does not cover it.

---

## Mode A: stock Kindle (browser only)

1. On the server, create a device token and put it in `DEVICE_TOKENS`.
2. On the Kindle, open **Menu → Experimental Browser**.
3. Open one of these pages and bookmark it:
   - **Image mode (recommended):** `https://dash.example.com/dashboard/image?token=YOUR_DEVICE_TOKEN`. The server renders the whole page to one PNG, so the old browser only has to show an image. **Tap the image or "Tap to refresh"** to get a new one.
   - **HTML mode:** `https://dash.example.com/dashboard?token=YOUR_DEVICE_TOKEN`. This is the same layout as plain HTML/CSS, with no JavaScript, flexbox or grid. Use the **Refresh** link at the bottom.
4. Optional: add `&refresh=900` to ask for a `<meta http-equiv="refresh">` reload every 15 minutes. Pages include it by default: `html_refresh_seconds: 900` in `config.yaml`, or `&refresh=0` to turn it off.

### Limitations (Mode A)

- **There is no reliable background update.** The stock Kindle sleeps after about 10 minutes and shows the screensaver. While it sleeps, Wi-Fi turns off and the browser stops, so a meta refresh only fires while the device is awake. The stock firmware has no supported way to stop sleep.
- The experimental browser runs an old WebKit, which is why the page avoids JavaScript and modern CSS. Its fonts and line wrapping may differ slightly from the PNG.
- **TLS:** older firmware may not connect to servers that only accept newer TLS versions or ciphers. If HTTPS fails, try a newer firmware, use Mode B, or serve plain HTTP only on a trusted LAN or VPN. Do not expose plain HTTP with a token on the internet.
- The token is in the URL, so it is stored in the browser history and bookmarks. Use a token only for this device and rotate it if the device is lost.
- None of this has been tested on real PW3 hardware by this project.

---

## Mode B: jailbroken Kindle with FBInk

### Prerequisites (you install these yourself)

- A jailbroken PW3 with **KUAL** installed.
- **FBInk**: the script finds the `fbink` binary in `PATH`, `/mnt/us/libkh/bin`, `/mnt/us/usbnet/bin`, the MRInstaller location, or `extensions/viaiz-dash/bin/fbink`.
- Optional: `curl` on the device. Without it, busybox `wget` is used and the token goes in the query string.

> New to this? **[JAILBREAK.md](JAILBREAK.md)** walks through jailbreaking the PW3
> (WinterBreak), installing KUAL + MRPI, and getting an `fbink` binary onto the device —
> the third-party steps this project assumes you've already done. It's version-dependent,
> so follow the live upstream pages it links; do it at your own risk.

### Install — option A: the script (needs a shell)

1. Connect the Kindle by USB so it mounts as a drive.
2. From this repository, run:

   ```bash
   ./kindle/install.sh /media/$USER/Kindle
   ```

   On Windows Git Bash use `/d` or your drive letter, and on macOS `/Volumes/Kindle`.

   The script only copies files into `extensions/viaiz-dash/` and refuses to run if the volume does not look like a Kindle that has KUAL.
3. Edit `extensions/viaiz-dash/dash.conf` on the Kindle drive. At minimum, set `SERVER_URL` and `DEVICE_TOKEN`.
4. Eject the Kindle. Open **KUAL → ViaiZ Dashboard → Show one frame now** to test.
5. If the frame looks right, choose **Start dashboard**.

### Install — option B: the drop-in zip (just copy folders)

If the machine with the Kindle attached has no shell (or you'd rather not clone the
repo there), build a self-contained zip once and copy it over:

```bash
go run ./cmd/mkrelease      # -> dist/viaiz-dash-<version>.zip
```

The zip holds `INSTALL.txt` plus a ready `viaiz-dash/` folder, with the scripts already
marked executable (0755) and LF line endings. On the target machine:

1. Open the zip and read `INSTALL.txt`.
2. Copy the `viaiz-dash` folder into the Kindle's `extensions/` folder, giving
   `<Kindle>/extensions/viaiz-dash/`.
3. On the Kindle drive, copy `dash.conf.example` to `dash.conf` and set `SERVER_URL` and
   `DEVICE_TOKEN`.
4. Eject, then **KUAL → ViaiZ Dashboard → Show one frame now**, and **Start dashboard**.

Both options install exactly the same files; the zip just needs no tools beyond a file
copy and an unzip.

### Settings (`dash.conf`)

| Key | Default | Meaning |
|---|---|---|
| `INTERVAL` | `900` | Seconds between refreshes (15 minutes) |
| `PAGE` | `main` | Which configured page to show |
| `FULL_REFRESH_EVERY` | `4` | Do a full flashing GC16 refresh every N updates to clear ghosting. The updates in between are non-flashing |
| `WIFI_TIMEOUT` | `60` | Seconds to wait for Wi-Fi after enabling it |
| `WIFI_OFF_BETWEEN` | `no` | Turn Wi-Fi off between updates to save power |
| `SLEEP_MODE` | `awake` | `awake` keeps the device awake with the screensaver blocked. `suspend` is **experimental**: it sets an RTC wake alarm and suspends to RAM |
| `STOP_FRAMEWORK` | `no` | Stop `lab126_gui` so the Kindle UI cannot draw over the dashboard. It is restored on stop |

### How it behaves

- **Each cycle:** check Wi-Fi with `lipc-get-prop com.lab126.wifid cmState` and enable it if needed. Download the PNG and check its PNG header and size. Only then replace the image and draw it with `fbink -g`.
- **On failure:** the last good image stays on screen and a one-line note appears at the bottom ("Offline - showing data from HH:MM"). The script retries every minute for 3 attempts, then goes back to the normal interval. The image itself also carries the server-side "Last sync" time and stale markers.
- **Power:** `awake` mode sets `preventScreenSaver 1` and sleeps between cycles, which is the most reliable option. Expect days, not weeks, on battery, so keep the Kindle on a charger. Turning on `WIFI_OFF_BETWEEN` saves power at the cost of reconnecting each cycle.
- **Stop:** **KUAL → Stop dashboard** kills the loop and restores the screensaver (and the UI, if it was stopped). A reboot also returns everything to normal, because nothing is installed outside the extension folder.
- **Logs:** `extensions/viaiz-dash/log/dash.log`, trimmed automatically.

### Not verified on hardware

The loop logic was tested in a simulation with stubbed `fbink` and `lipc`: refresh cadence, full-refresh rotation, failure overlay and cleanup on SIGTERM. None of the following has been tested on a real PW3:

- FBInk waveform flags (`-W GC16`) on your FBInk build
- `lipc` property names on your firmware
- `stop lab126_gui` on newer firmware
- **`SLEEP_MODE=suspend`**: RTC wake may not work on every firmware, and the device might not wake until you press the power button

Test with **Show one frame now** first, then run the loop while you watch the log.

### Uninstall

1. **KUAL → ViaiZ Dashboard → Stop dashboard** (or reboot the Kindle).
2. Connect it by USB and run:

   ```bash
   ./kindle/uninstall.sh /media/$USER/Kindle
   ```

   The script asks before deleting `extensions/viaiz-dash`.

---

## Reducing ghosting

- Keep `FULL_REFRESH_EVERY` between 3 and 6. Lower values give a cleaner image but flash more often.
- The frame is quantized to exactly 16 gray levels to match the panel. Dithering is off for sharp text. Set `render.dither: true` only if you add photos.
- The layout uses solid black and white with thick strokes, and no gradients, so partial updates stay clean.
