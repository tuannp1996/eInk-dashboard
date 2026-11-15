# Preparing a Kindle Paperwhite 3: jailbreak + KUAL + FBInk

Mode B of this project (see [KINDLE.md](KINDLE.md)) needs a Kindle that is already
**jailbroken**, with **KUAL** and **FBInk** installed. This document explains how to
get there. It uses third-party community tools — **this project does not ship, bundle
or automate any of them**, and none of this touches Amazon's servers or firmware on your
behalf. You do each step yourself, on hardware you own, at your own risk.

> ⚠️ **Read first.**
> - Jailbreaking is **not endorsed by Amazon** and can **void your warranty**.
> - A wrong firmware package or an interrupted write can **brick the device**. The risk
>   is low with the current tools, but it is not zero.
> - Jailbreaking **blocks future OTA firmware updates** (on purpose — an update can remove
>   the jailbreak). You choose to stay on your current firmware.
> - **Everything below is version-dependent and changes over time.** Treat the exact
>   filenames, versions and URLs here as a snapshot (checked November 2025). Always follow
>   the **live** instructions on the linked pages, which are maintained; this file is only
>   a map so you know what the steps are and how they feed into this project.
> - This is for the **Paperwhite 3 (PW3 / 7th gen, 2015, model codes G090…)**. Other models
>   differ.

---

## 0. Identify your device and firmware

You need the exact firmware version before anything else — it decides whether a jailbreak
exists for you.

1. On the Kindle: **Home → Menu (⋮) → Settings**, then **Menu (⋮) → Device Info**
   (the firmware version also appears at the bottom of the main Settings screen).
2. Note the version, e.g. `5.16.2.1.1`.
3. Confirm the model is a **Paperwhite 3** (7th gen). Settings → Device Info shows the model.

**Which jailbreak for which firmware** (KindleModding project, late 2025):

| Firmware | Tool | Section |
|---|---|---|
| **≤ 5.18.0.2** | **WinterBreak** | [§2](#2-jailbreak-with-winterbreak) |
| **5.18.1 – 5.18.5.0.1** | **AdBreak** (needs lockscreen ads) | [§2b](#2b-jailbreak-with-adbreak-firmware-5181--5185) |
| **5.18.5.0.1+** | none yet | use **Mode A** |

- **WinterBreak** works on any supported model up to and including `5.18.0.1.0.1`.
- **AdBreak** (released Sept 2025) covers `5.18.1 – 5.18.5.0.1`, but only on a **registered,
  ad-supported** Kindle (one that shows "Special Offers" on the lockscreen), and **not** on
  the **Scribe** or **Colorsoft**.
- These change over time. Always confirm your exact model + firmware with the official
  **Jailbreak Wizard** before starting: <https://kindlemodding.org/jailbreak-wizard.html>

> This device (checked on the attached Kindle): firmware **`5.18.1.1.1`** → in the
> **AdBreak** range. Follow **[§2b](#2b-jailbreak-with-adbreak-firmware-5181--5185)**,
> not §2. If your model turns out to be a Scribe/Colorsoft, or has no ads and ads can't be
> enabled, AdBreak won't apply — use **Mode A** instead.

---

## 1. Prevent auto-updates (do this before and after)

An OTA update can silently bump you past the jailbreakable firmware, or remove the
jailbreak later. Turn Wi-Fi off / enable Airplane mode while preparing, and follow the
official guide to block updates:
<https://kindlemodding.org/jailbreaking/prevent-auto-update/>

The jailbreak guide also has you temporarily **fill the device's storage** so an update
can't download mid-process; you delete the filler files afterwards.

---

## 2. Jailbreak with WinterBreak

WinterBreak is maintained by the KindleModding project. Full, current instructions:
<https://kindlemodding.org/jailbreaking/WinterBreak/>

The flow (as of v2.1.0) is roughly:

1. Fill storage (step 1 above) so an update can't interrupt.
2. Download the latest release:
   <https://github.com/KindleModding/WinterBreak/releases/latest/download/WinterBreak.tar.gz>
3. Turn on **Airplane mode**, then **reboot** the Kindle.
4. Connect by USB and **extract `WinterBreak.tar.gz` on your PC first**, then copy the
   extracted files to the Kindle root, replacing any files it prompts for.
   On Linux/macOS make sure the hidden `.active_content_sandbox` folder is copied too.
5. Eject. Open the **Kindle Store** (cart icon). When asked, allow it to turn Airplane
   mode off. The **Mesquito** launcher loads.
6. Tap the **WinterBreak** icon and wait ~30 s. The screen shows some text and the UI
   restarts. (If nothing happens, retry — the page has a troubleshooting section.)
7. Delete the filler files and any leftover `.bin` files in the Kindle root.

> **Persistence:** a bare jailbreak may not survive every reboot on its own. Install a
> **hotfix** so it sticks and re-asserts itself after an update:
> <https://kindlemodding.org/jailbreaking/post-jailbreak/setting-up-a-hotfix/>
> You copy an `Update_..._hotfix_*_install.bin` to the Kindle root and let it run; re-run
> it after any future OTA.

---

## 2b. Jailbreak with AdBreak (firmware 5.18.1 – 5.18.5)

Use this instead of §2 if you're on `5.18.1`–`5.18.5.0.1` (this project's attached device,
`5.18.1.1.1`, is here). AdBreak is maintained by the KindleModding project. **Follow the
live page — it has the exact mechanics and troubleshooting; the summary below is just the
shape of it:** <https://kindlemodding.org/jailbreaking/AdBreak/>

**Extra prerequisites (on top of §0):**

- The Kindle is **registered** to an Amazon account and not blacklisted.
- It is an **ad-supported** model — it shows **Special Offers** on the lockscreen. If yours
  is ad-free, some accounts can switch Special Offers on at
  <https://www.amazon.com/myx> (Devices) temporarily, then off again afterwards.
- It is **not** a Scribe or Colorsoft.

**Confirm ads are present (the trigger depends on them):**

1. Connect Wi-Fi and leave the Kindle on a while so it downloads ads; press the power
   button — an ad should appear on the lockscreen. (If none ever appear, a factory reset
   can prompt them.)
2. Once ads show, enable **Airplane mode**.
3. Lockscreen menu → **View all ads** → confirm several "special offers" are listed.

**Install (the interactive part is yours — it happens on the Kindle screen):**

1. Download the latest release `adbreak.zip`:
   <https://github.com/KindleModding/AdBreak/releases/latest/download/adbreak.zip>
2. Connect by USB. Copy the Kindle's **`system/.assets`** folder to your PC.
3. Extract AdBreak **into that copied `.assets` folder** and run its replacement step
   (`replace.bat` on Windows; the `find … -exec` command on macOS/Linux — both are on the
   page).
4. Delete the original `system/.assets` on the Kindle and copy your modified folder back
   in its place. Eject.
5. On the Kindle: **tap an ad** and work through the pop-ups. After you close the final
   pop-up the jailbreak script runs. ("Application error" pop-ups are harmless.)
6. Afterwards, delete any leftover `.bin` update files in the Kindle root. Updates are
   blocked automatically (also redo §1's prevent-auto-update).

> **Persistence:** install a hotfix so the jailbreak survives reboots/updates, same idea as
> §2: <https://kindlemodding.org/jailbreaking/post-jailbreak/setting-up-a-hotfix/>

Then continue to **§3** (KUAL + MRPI) — that part is the same for both jailbreaks.

---

## 3. Install KUAL + MRPI

**KUAL** (Kindle Unified Application Launcher) is the menu this project's extension plugs
into. **MRPI** (MobileRead Package Installer) installs `.bin` packages via the search bar.
They depend on each other — install both together.

Current instructions:
<https://kindlemodding.org/jailbreaking/post-jailbreak/installing-kual-mrpi/>
Packages come from **NiLuJe's snapshot thread**:
<https://www.mobileread.com/forums/showthread.php?t=225030>

The flow:

1. Pick the KUAL variant for your firmware. For PW3 on **FW 5.9+**, use the
   **KUAL Booklet (coplate)** build (`KUALBooklet_<hash>_install.bin`). For 5.8.x and
   older, the plain KUAL Booklet (`KUALBooklet_v*_install.bin`).
2. Connect by USB. Copy MRPI's **`extensions/`** and **`mrpackages/`** folders to the
   **root** of the Kindle's visible storage (next to `documents/`).
3. Copy the chosen `KUALBooklet_*_install.bin` into **`mrpackages/`**.
4. Eject the Kindle. In the Kindle's search bar type **`;log mrpi`** and run it.
   `Hush, little baby...` appears at the bottom of the screen while it installs.
   (Fallback if `;log` is disabled: copy an `Update_KUALBooklet_hotfix_*_install.bin`
   to the root and run **Settings → Update Your Kindle**.)
5. You may need to free **~220 MB** to install both.

When it finishes, open **KUAL** from the Kindle home screen (it appears as a book/app).
An empty KUAL menu that opens = success.

---

## 4. Install FBInk

**FBInk** (FrameBuffer eInker, by NiLuJe) is what draws the dashboard PNG onto the e-ink
panel. The dashboard loop (`bin/viaiz-dash.sh`) looks for an `fbink` binary in these
places, in order:

| Path | Comes from |
|---|---|
| `fbink` on `PATH` | USBNetwork shell profile, or a symlink you add |
| `/mnt/us/libkh/bin/fbink` | KindleHelper / `libkh` |
| `/mnt/us/usbnet/bin/fbink` | the **USBNetwork** hack (ships FBInk) |
| `/mnt/us/extensions/MRInstaller/bin/KHF/fbink` | MRInstaller |
| `/mnt/us/extensions/viaiz-dash/bin/fbink` | **you drop it here** (simplest) |

### Easiest: drop the binary next to this extension

1. Download the latest FBInk release (prebuilt static binaries, includes a Kindle build):
   <https://github.com/NiLuJe/FBInk/releases/latest> — e.g. `FBInk-v1.25.0.tar.xz`.
   (It's also mirrored in NiLuJe's MobileRead thread:
   <https://www.mobileread.com/forums/showthread.php?t=299110>.)
2. Extract it on your PC. PW3 is **32-bit ARM (armv7)** — use the **Kindle** build's
   `fbink` binary. (The archive ships one static binary per device family; the README
   inside names them. If in doubt, the Kindle/armv7 one.)
3. With the Kindle connected by USB, copy that `fbink` into
   `extensions/viaiz-dash/bin/` on the Kindle (create the folder if the ViaiZ extension
   isn't installed yet — see [KINDLE.md](KINDLE.md) §Mode B → Install).
4. It must be **executable**. `install.sh` from this repo copies scripts but not an
   arbitrary `fbink`; set the bit from a device shell once:
   ```sh
   chmod +x /mnt/us/extensions/viaiz-dash/bin/fbink
   ```
   (Or install USBNetwork, which both gives you that shell and puts `fbink` at
   `/mnt/us/usbnet/bin/fbink`, which the script also finds — no `chmod` needed.)

### Alternative: it may already be there

FBInk ships inside several common hacks. If you install **USBNetwork** (recommended
anyway — it's how you get an SSH/serial shell to test), FBInk lands at
`/mnt/us/usbnet/bin/fbink` and the script picks it up automatically. **KOReader** also
bundles FBInk, but at its own path, so prefer one of the paths in the table above.

### Verify FBInk

From a device shell (USBNetwork SSH, or KUAL + a terminal extension):
```sh
/mnt/us/extensions/viaiz-dash/bin/fbink -h        # prints usage = binary runs on this CPU
/mnt/us/extensions/viaiz-dash/bin/fbink "hello"    # should print on the top-left of the screen
```
If `-h` fails with "not executable" or a format error, you have the wrong architecture
build — use the Kindle/armv7 one.

---

## 5. Stock tools the loop also uses (nothing to install)

The dashboard loop calls these, which are **already present** on jailbroken PW3 firmware —
you do not install them:

- `lipc-get-prop` / `lipc-set-prop` — Wi-Fi state and screensaver control
- `gasgauge-info` — battery percentage
- `stop` / `start lab126_gui` — only used if you set `STOP_FRAMEWORK=yes`
- busybox `wget` — fallback when `curl` is absent

If you want `curl` (so the device token goes in a header instead of the URL), install it
from NiLuJe's coreutils/busybox snapshot in the same MobileRead thread as KUAL. Optional.

---

## 6. Now install the dashboard

Once **KUAL opens** and **`fbink -h` works**, you're on the Mode B prerequisites. Continue
at [KINDLE.md](KINDLE.md) → **Mode B → Install**:

```bash
./kindle/install.sh /media/$USER/Kindle    # or /d on Windows Git Bash, /Volumes/Kindle on macOS
```

Then edit `dash.conf` (`SERVER_URL`, `DEVICE_TOKEN`) and use
**KUAL → ViaiZ Dashboard → Show one frame now**.

> **Panel size:** this project renders **1072×1448** (the PW3 panel). A device on 5.18.x is
> usually a newer model — if yours has a different panel (e.g. a PW5 at 1236×1648), set
> `render.width` / `render.height` in `config.yaml` to the panel's real pixels so FBInk
> draws it sharp and full-screen. Check your model's resolution first.

---

## 7. Reverting

- **The dashboard extension:** `./kindle/uninstall.sh <mount>` (or just delete
  `extensions/viaiz-dash`). It never touched anything else.
- **The jailbreak / KUAL:** removed by a **factory reset** (Settings → Menu → Reset), or
  by letting the device take an OTA update (which is why updates are blocked). FBInk and
  KUAL live under `/mnt/us`, so deleting their folders removes them, but the jailbreak
  itself is cleared by the reset. Follow the KindleModding uninstall notes for your tools.

---

## Sources (checked November 2025 — verify the live pages)

- KindleModding — WinterBreak: <https://kindlemodding.org/jailbreaking/WinterBreak/>
- KindleModding — Jailbreak Wizard: <https://kindlemodding.org/jailbreak-wizard.html>
- KindleModding — Installing KUAL & MRPI: <https://kindlemodding.org/jailbreaking/post-jailbreak/installing-kual-mrpi/>
- KindleModding — Setting up a hotfix: <https://kindlemodding.org/jailbreaking/post-jailbreak/setting-up-a-hotfix/>
- KindleModding — Prevent auto-update: <https://kindlemodding.org/jailbreaking/prevent-auto-update/>
- WinterBreak releases: <https://github.com/KindleModding/WinterBreak/releases>
- KindleModding — AdBreak (FW 5.18.1–5.18.5): <https://kindlemodding.org/jailbreaking/AdBreak/>
- AdBreak releases + model notes (GitHub): <https://github.com/KindleModding/AdBreak>
- NiLuJe's KUAL / MRPI / tools snapshots (MobileRead): <https://www.mobileread.com/forums/showthread.php?t=225030>
- FBInk (NiLuJe) releases: <https://github.com/NiLuJe/FBInk/releases>
- FBInk MobileRead thread: <https://www.mobileread.com/forums/showthread.php?t=299110>
- MobileRead 5.x Jailbreak wiki (legacy/background): <https://wiki.mobileread.com/wiki/5_x_Jailbreak>
