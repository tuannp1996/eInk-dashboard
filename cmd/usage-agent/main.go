// Command usage-agent runs on the workstation where Claude Code / Codex CLI
// are used. It never reads or sends credentials: only percentages and reset
// timestamps that the tools themselves expose locally.
//
//	usage-agent statusline              # configure as Claude Code statusLine command
//	usage-agent push --server URL       # push snapshots (token from VIAIZ_INGEST_TOKEN)
//	usage-agent push --server URL --once --dry-run
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/viaiz/eink-dashboard/internal/agent"
	"github.com/viaiz/eink-dashboard/internal/dashboard"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "statusline":
		statusline(os.Args[2:])
	case "push":
		push(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  usage-agent statusline [--out FILE]
  usage-agent push --server https://dash.example.com [--interval 5m] [--once] [--dry-run]
                   [--claude-file FILE] [--codex-home DIR] [--no-claude] [--no-codex]
  Ingest token is read from the VIAIZ_INGEST_TOKEN environment variable.`)
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func defaultClaudeFile() string { return filepath.Join(home(), ".viaiz", "claude-usage.json") }

func defaultCodexHome() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	return filepath.Join(home(), ".codex")
}

// statusline is invoked by Claude Code with session JSON on stdin. It must be
// fast and must always print something; errors are swallowed.
func statusline(args []string) {
	fs := flag.NewFlagSet("statusline", flag.ExitOnError)
	out := fs.String("out", defaultClaudeFile(), "where to store the latest rate-limit snapshot")
	_ = fs.Parse(args)
	b, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	snap, model, err := agent.ParseClaudeStatusline(b, time.Now())
	if err == nil && snap != nil {
		_ = agent.WriteJSONAtomic(*out, snap)
	}
	fmt.Println(agent.StatusLineText(model, snap))
}

func push(args []string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	server := fs.String("server", os.Getenv("VIAIZ_SERVER"), "dashboard base URL")
	interval := fs.Duration("interval", 5*time.Minute, "push interval")
	once := fs.Bool("once", false, "push once and exit")
	dry := fs.Bool("dry-run", false, "print reports instead of sending")
	claudeFile := fs.String("claude-file", defaultClaudeFile(), "snapshot written by `usage-agent statusline`")
	codexHome := fs.String("codex-home", defaultCodexHome(), "Codex home (contains sessions/)")
	noClaude := fs.Bool("no-claude", false, "disable Claude Code source")
	noCodex := fs.Bool("no-codex", false, "disable Codex source")
	_ = fs.Parse(args)

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	token := os.Getenv("VIAIZ_INGEST_TOKEN")
	if !*dry && (*server == "" || token == "") {
		log.Error("--server (or VIAIZ_SERVER) and VIAIZ_INGEST_TOKEN are required unless --dry-run")
		os.Exit(2)
	}
	host, _ := os.Hostname()
	if len(host) > 64 {
		host = host[:64]
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	sent := map[string]time.Time{}
	cycle := func() {
		var reps []dashboard.UsageReport
		if !*noClaude {
			if s, err := agent.ReadClaudeSnapshot(*claudeFile); err == nil {
				reps = append(reps, agent.ClaudeReport(s, host))
			} else {
				log.Info("claude: no snapshot yet (is the statusLine configured and a session running?)", "file", *claudeFile)
			}
		}
		if !*noCodex {
			if p, err := agent.LatestRollout(filepath.Join(*codexHome, "sessions")); err == nil {
				if f, err := os.Open(p); err == nil {
					rep, perr := agent.ParseCodexRollout(f, host)
					f.Close()
					switch {
					case perr != nil:
						log.Warn("codex: parse failed", "file", p, "err", perr)
					case rep == nil:
						log.Info("codex: latest rollout has no rate_limits event", "file", p)
					default:
						reps = append(reps, *rep)
					}
				}
			} else {
				log.Info("codex: no rollout files", "err", err)
			}
		}
		for _, r := range reps {
			if !*dry && !sent[r.Provider].Before(r.ObservedAt) {
				continue // unchanged since last successful push
			}
			if *dry {
				b, _ := json.MarshalIndent(r, "", "  ")
				fmt.Println(string(b))
				continue
			}
			if err := send(ctx, *server, token, r); err != nil {
				log.Warn("push failed", "provider", r.Provider, "err", err)
				continue
			}
			sent[r.Provider] = r.ObservedAt
			log.Info("pushed", "provider", r.Provider, "observed_at", r.ObservedAt)
		}
	}
	cycle()
	if *once {
		return
	}
	t := time.NewTicker(*interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cycle()
		}
	}
}

func send(ctx context.Context, server, token string, r dashboard.UsageReport) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			}
		}
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		req, _ := http.NewRequestWithContext(cctx, http.MethodPost, strings.TrimRight(server, "/")+"/api/ingest/usage", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		cancel()
		if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
			return nil
		}
		lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr // do not retry client errors
		}
	}
	return lastErr
}
