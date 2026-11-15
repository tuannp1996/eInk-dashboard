// Command mkrelease packages the ViaiZ KUAL extension into a single zip that
// can be dropped straight onto a jailbroken Kindle — no repo checkout or
// install.sh needed on the machine with the Kindle attached.
//
//	go run ./cmd/mkrelease                 # -> dist/viaiz-dash-<version>.zip
//	go run ./cmd/mkrelease -o build/x.zip  # custom output path
//
// The zip stores Unix permissions (scripts 0755, everything else 0644) and
// normalises text files to LF, so the scripts run correctly on the device
// even when built on Windows.
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Layout inside the zip:
//
//	INSTALL.txt          <- read this first
//	viaiz-dash/          <- copy this whole folder into <Kindle>/extensions/
//	  config.xml
//	  menu.json
//	  dash.conf.example
//	  bin/*.sh
func main() {
	src := flag.String("src", "kindle/viaiz-dash", "extension source directory")
	out := flag.String("o", "", "output zip path (default dist/viaiz-dash-<version>.zip)")
	flag.Parse()

	version := readVersion(filepath.Join(*src, "config.xml"))
	dest := *out
	if dest == "" {
		dest = filepath.Join("dist", "viaiz-dash-"+version+".zip")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		die(err)
	}

	// Collect files under src, deterministically.
	var files []string
	err := filepath.WalkDir(*src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := d.Name()
		// Never ship a real dash.conf (it may hold a token) or stray logs.
		if base == "dash.conf" || strings.HasPrefix(base, ".") {
			return nil
		}
		if strings.Contains(filepath.ToSlash(p), "/log/") {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		die(err)
	}
	sort.Strings(files)

	f, err := os.Create(dest)
	if err != nil {
		die(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)

	modTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // stable mtime for reproducible zips

	// INSTALL.txt at the zip root.
	writeEntry(zw, "INSTALL.txt", []byte(installText(version)), 0o644, modTime)

	for _, p := range files {
		rel, _ := filepath.Rel(*src, p)
		name := "viaiz-dash/" + filepath.ToSlash(rel)
		data, err := os.ReadFile(p)
		if err != nil {
			die(err)
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if isText(name) {
			data = []byte(strings.ReplaceAll(string(data), "\r\n", "\n"))
		}
		writeEntry(zw, name, data, mode, modTime)
	}

	if err := zw.Close(); err != nil {
		die(err)
	}
	info, _ := os.Stat(dest)
	fmt.Printf("wrote %s (%d bytes, %d files, version %s)\n", dest, info.Size(), len(files)+1, version)
}

func writeEntry(zw *zip.Writer, name string, data []byte, mode os.FileMode, modTime time.Time) {
	h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modTime}
	h.SetMode(mode)
	w, err := zw.CreateHeader(h)
	if err != nil {
		die(err)
	}
	if _, err := w.Write(data); err != nil {
		die(err)
	}
}

func isText(name string) bool {
	switch filepath.Ext(name) {
	case ".sh", ".xml", ".json", ".conf", ".example", ".txt", ".md":
		return true
	}
	return strings.HasSuffix(name, ".conf.example")
}

var verRe = regexp.MustCompile(`<version>([^<]+)</version>`)

func readVersion(configPath string) string {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return "0.0"
	}
	if m := verRe.FindSubmatch(b); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return "0.0"
}

func installText(version string) string {
	return "ViaiZ Dashboard - KUAL extension v" + version + `
=============================================

This package installs the e-ink dashboard client onto a Kindle that is ALREADY
jailbroken, with KUAL and FBInk installed. If your Kindle is not jailbroken yet,
see docs/JAILBREAK.md in the project first.

INSTALL (no computer tools needed beyond copying files)
-------------------------------------------------------
1. Connect the Kindle by USB so it shows up as a drive.
2. Copy the "viaiz-dash" folder from this zip into the Kindle's
   "extensions" folder, so you end up with:
       <Kindle>/extensions/viaiz-dash/
   (The "extensions" folder exists once KUAL is installed.)
3. On the Kindle drive, open  extensions/viaiz-dash/  and copy
   "dash.conf.example" to "dash.conf".
4. Edit "dash.conf" and set at least:
       SERVER_URL   = https://your-server
       DEVICE_TOKEN = one of DEVICE_TOKENS on the server
5. FBInk: if "fbink" is not already on the device (via USBNetwork at
   /mnt/us/usbnet/bin/fbink, libkh, or MRInstaller), put the Kindle/armv7
   "fbink" binary into  extensions/viaiz-dash/bin/  and make it executable:
       chmod +x /mnt/us/extensions/viaiz-dash/bin/fbink
   See docs/JAILBREAK.md section 4.
6. Eject the Kindle. On the device open:
       KUAL -> ViaiZ Dashboard -> Show one frame now
   If the frame looks right, choose "Start dashboard".

UNINSTALL
---------
KUAL -> ViaiZ Dashboard -> Stop dashboard, then delete the
extensions/viaiz-dash folder from the Kindle. Nothing is installed anywhere
else; a reboot restores normal behaviour.

Full documentation: docs/KINDLE.md and docs/JAILBREAK.md in the project.
`
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "mkrelease:", err)
	os.Exit(1)
}
