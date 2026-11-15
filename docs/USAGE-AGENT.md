# Usage agent (Claude Code and Codex quotas)

Claude Code and Codex do **not** provide a public API for subscription quota. The server never logs in to either service. Instead, a small agent runs on the machine where you use these tools. It reads values the tools already expose locally and pushes **only the numbers** to the dashboard: used %, reset time and window length. It never reads or sends tokens, cookies or credential files.

Build it on your workstation:

```bash
go build -o usage-agent ./cmd/usage-agent
```

It is also included in the Docker image at `/app/usage-agent`.

---

## Claude Code

Source: the documented **statusLine** JSON (https://code.claude.com/docs/en/statusline). Claude Code runs your status line command with session JSON on stdin. For claude.ai Pro and Max subscribers, after the first API response of a session, that JSON contains:

```json
"rate_limits": {
  "five_hour": { "used_percentage": 23.5, "resets_at": 1738425600 },
  "seven_day": { "used_percentage": 41.2, "resets_at": 1738857600 }
}
```

`resets_at` is in Unix seconds. API-key users get no `rate_limits`; the Claude
card stays hidden until a usage report exists.

**Setup.** Add this to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/usage-agent statusline"
  }
}
```

The command prints a short status line, for example `Opus · 5h 24% · 7d 41%`. It also writes the latest snapshot to `~/.viaiz/claude-usage.json` (mode 0600). It makes no network calls, so it stays fast.

This replaces any status line you already have. To keep yours, call `usage-agent statusline` from your own script and pass it the same stdin.

**Limitation:** data only updates while a Claude Code session is running. Between sessions the dashboard keeps showing the last snapshot with "Reported N min ago". After 6 hours it is marked **STALE**. Once a window's reset time has passed, the old percentage is hidden and shown as "?".

## Codex CLI

Source: Codex CLI session rollout files, `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` (or `$CODEX_HOME/sessions`). Codex records `token_count` events carrying a `rate_limits` object:

```json
{"timestamp":"…","type":"event_msg","payload":{"type":"token_count",
 "rate_limits":{"primary":{"used_percent":12.0,"window_minutes":300,"resets_in_seconds":9000},
                "secondary":{"used_percent":66.0,"window_minutes":10080,"resets_at":1795000000}}}}
```

The agent takes the **last** such event from the newest rollout file and timestamps it with the event's own time, not the current time. Windows are mapped by `window_minutes`: 300 is the 5-hour window and 10080 is weekly. If `window_minutes` is missing, primary is treated as 5-hour and secondary as weekly.

This file format may change between Codex versions. The parser was checked
against real local session events on 2026-11-15, including `primary` (300 minutes)
and `secondary` (10080 minutes) windows with Unix `resets_at` timestamps. If no
event is found, nothing is reported and the widget stays hidden until a report exists.
No values are guessed. Check the output on your machine with `--dry-run` first.

## Production Compose

On the viaiz.com host, the production overlay includes a `usage-agent` service:

```bash
docker compose -f docker-compose.yml -f docker-compose.production.yml up -d --no-build --wait
docker compose -f docker-compose.yml -f docker-compose.production.yml logs --tail=20 usage-agent
```

The agent reads the host's Codex sessions via a read-only bind mount and reports
every minute to `http://dashboard:8080/elink-dashboard` on a private network.
It takes the first token from `INGEST_TOKENS` in `.env`. The dashboard uses the
existing shared Redis. No Codex credential files are mounted.

Set `CODEX_SESSIONS_DIR`, `USAGE_AGENT_UID` and `USAGE_AGENT_GID` in `.env` if the
host's session directory or file owner differs from `/home/futu/.codex/sessions`
and 1000:1000. The host must be the machine where Codex sessions are written.
This service enables Codex only; Claude still requires its statusLine snapshot
integration described above.

If the Codex card is missing, check that the service is running, the mount contains
recent rollout files, and the logs contain `pushed` for provider `codex`.
Running `/app/usage-agent` inside the dashboard container alone cannot find the
host's sessions: that container's home is `/tmp` and it has no session mount.

## Pushing to the server

Test locally without sending anything:

```bash
./usage-agent push --once --dry-run
```

Run continuously, pushing every 5 minutes and only when a snapshot changed:

```bash
export VIAIZ_INGEST_TOKEN=<one of INGEST_TOKENS>
./usage-agent push --server https://dash.example.com --interval 5m
```

Useful flags: `--no-claude`, `--no-codex`, `--claude-file`, `--codex-home`.

To run it in the background, use a systemd user unit on Linux, a launchd agent on macOS, or Task Scheduler on Windows (at logon: `usage-agent push --server …`, with `VIAIZ_INGEST_TOKEN` set in the user environment).

Example systemd unit, `~/.config/systemd/user/viaiz-usage.service`:

```ini
[Unit]
Description=ViaiZ usage agent
[Service]
Environment=VIAIZ_INGEST_TOKEN=xxxxxxxx
ExecStart=%h/bin/usage-agent push --server https://dash.example.com --interval 5m
Restart=always
[Install]
WantedBy=default.target
```

Enable it:

```bash
systemctl --user enable --now viaiz-usage
```

## Ingest API

`POST /api/ingest/usage` with `Authorization: Bearer <ingest token>`:

```json
{
  "provider": "claude",
  "origin": "claude-statusline",
  "host": "laptop",
  "observed_at": "2026-11-15T13:49:47Z",
  "five_hour": { "used_percent": 23.5, "resets_at": 1794757786, "window_minutes": 300 },
  "weekly":    { "used_percent": 41,   "resets_at": 1795050586, "window_minutes": 10080 }
}
```

The server checks that `provider` is `claude` or `codex`, that `used_percent` is between 0 and 100, and that `observed_at` is not in the future. It rejects unknown fields. An older snapshot never overwrites a newer one. Any other tool that can produce this payload can feed the dashboard too.
