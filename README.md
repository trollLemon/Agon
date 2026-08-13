# goagentdisc — two-agent debates in your terminal (Go)

Pit two AI agents against each other over **any** proposition and get a reasoned verdict.
The proposition can be a design/architecture choice, a technical tradeoff, a strategy or
policy, a factual claim, or a head-to-head **X vs Y** comparison.

goagentdisc is a single, self-contained terminal application. There is no server, no MCP
tool, and no external harness: it loads a local model in-process via the
[kronk](https://github.com/ardanlabs/kronk) SDK and runs the whole debate — advocate,
critic, and a neutral judge — inside one binary, streaming the transcript to a
[Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI as it happens.

## Quick start

```bash
go run ./cmd/goagentdisc
```

This opens the TUI at the main menu. There are no flags: everything (topic, starting
context, mode, tone, rounds, model) is entered on the new-debate form.

- **Start a new debate** — fill in the topic and (optionally) freeform starting context
  (paste a repo path and a change description here to ground the debate in real code),
  pick a mode/tone/round count, and submit. The first debate triggers a one-time model
  download/load (progress shown on the form); the model is cached for later runs. The
  model field defaults to `Qwen/Qwen3-8B-Q8_0` (~8.7GB) — a meaningfully stronger
  instruction-follower than the smaller 0.6B/1.7B tiers, which are prone to ignoring
  structural prompt constraints (e.g. drafting multiple rounds at once). If you have
  the RAM to spare, type in a bigger model instead (e.g. `unsloth/gpt-oss-20b-Q8_0`
  at ~12GB) for even better quality.
- **Browse archive** — reopen any past debate, read-only.

Archived debates are written to `debates/<session_id>.json` (one file per session,
written exactly once when the verdict completes — a killed or crashed run leaves no
file).

## Screens & keys

- **Menu** — `↑/↓` select · `enter` confirm · `ctrl+c` quit.
- **New-debate form** — `tab`/`shift+tab` move between fields · `enter` next field (or
  submit from the last field) · `ctrl+s` submit · `esc` cancel.
- **Session view** (live or archived; exclusive full-screen — no menu chrome while a
  debate is open) — `j/k` or `↑/↓` scroll the transcript · `a` abort the live debate
  (with confirmation; discards the in-memory transcript, nothing is archived) · `esc`
  back to the menu (a live debate keeps running in the background; reopen it from the
  archive list, marked `●`, to resume watching) · `ctrl+c` quit the app.
- **Archive list** — `↑/↓` select · `enter` open (read-only) · `esc` back to the menu.

Only one debate can be live at a time; starting a new one while another is running is
blocked until you finish or abort it.

## Debate modes

- **`proposition`** (default) — one claim. `advocate` argues FOR, `critic` argues AGAINST.
- **`versus`** — two named options compete; each side advocates its own option.

## Tones

`formal` (default), `informal`, `genz`, `unhinged`. Tone is **style only** — it never lowers
the bar on substance, evidence, or honesty, and both sides share one tone so the debate stays
even.

## Evidence & code debates

Debaters back claims with **citations rendered as clickable markdown links** in the
transcript — `[label](https://…)` for external facts, `[path:line](path#Lline)` for code.
If the starting context contains a path that resolves to an existing directory, both
sides get sandboxed, read-only tool access (`read_file`, `grep`, `list_dir`) to that repo
so they can ground and cite real code; tool calls appear inline as collapsible `⚙` blocks
and are recorded in the archive alongside the messages.

## Layout

```
cmd/goagentdisc/        # entrypoint: bootstrap only, opens the TUI
internal/tui/           # bubbletea screens: menu, form, session view, archive list
internal/orchestrator/  # debate loop: roles, rounds, turn order, tool sub-loop, kronk adapter
internal/prompts/       # persona templates (advocate/critic/judge, modes, tones)
internal/tools/         # read_file, grep, list_dir — sandboxed to the detected repo
internal/archive/       # one-JSON-per-session: write-once, list, load
features/               # godog BDD feature files, driven via teatest
docs/                   # spec + architecture diagrams
```

## Testing

```bash
go test ./...                 # unit + BDD (features/), no model, no TTY, fast
go test ./... -race -p 1      # race detector; -p 1 serializes package binaries
                               # (recommended for -race: many concurrent -race
                               # binaries can starve each other's goroutines
                               # under real CPU contention and cause spurious
                               # timeouts in the teatest-driven tests)
go test ./features/...        # just the BDD scenarios (godog + teatest)
```

- `internal/orchestrator`, `internal/archive`, `internal/prompts`, `internal/tools` are
  unit-tested against a fake `ChatClient` — no real model, no TTY.
- `internal/tui` and `features/` drive the actual Bubble Tea app via
  [`teatest`](https://github.com/charmbracelet/x/tree/main/exp/teatest) with a fake
  `ChatClient`, so the whole app (menu → form → live debate → verdict → archive →
  browse → abort) is exercised without ever loading a real model.
- An integration test against a real, tiny kronk model (e.g. `unsloth/Qwen3-0.6B-Q8_0`)
  is planned per `docs/SPEC.md` D12 but not required for everyday development; it would
  be gated behind `-short` or an opt-in environment variable so it never runs by default.
