# agon 
> or *agony* if you dont have enough VRAM.

Pit two AI agents against each other over any proposition and get a reasoned verdict.
The proposition can be a design/architecture choice, a technical tradeoff, a strategy or
policy, or a head-to-head X vs Y comparison.

## Motivation
I had an idea for using two agents to debate over a topic and produce a verdict. My first implementation was a python based
MCP server you connected a harness to. I also tried this in go.

This worked, but I didn't like the idea of having to start the server each time, and connecting
the harness afterwards before I could use this feature.

I attended GopherconUK 2026, and got particularly excited about the [Kronk](https://github.com/ardanlabs/kronk) package: a go API that provided local inference with LLMs, 
with the intent to remove the need for a model server (or mcp server).

I wanted to experiment with using a spec document to guide an LLM on autopilot, rather than traditional
prompting in a harness, to move from a MCP server architecture to a TUI app. I had a spec document and had Kimi K3 refine it. The spec details the requirements, constraints, and UX design choices I wanted. 
I sent Sonnet 5 on the implementation.

The result worked... but was quite bloated. I was happy with the general direction but saw a lot of room for 
improvement in terms of design, maintainability, and code quality (you can see so in the commit history).

## Quick start

```bash
go run ./cmd
```

This opens the TUI at the main menu. There are no flags: everything (topic, starting
context, mode, tone, rounds, model) is entered on the new-debate form.

- **Start a new debate**: fill in the topic and (optionally) freeform starting context. You can specify
   paths to documents, code, etc, which will be added into the models read only sandbox.
- **Browse archive**: reopen any past debate, read-only.

Archived debates are written to `debates/<session_id>.json` (one file per session,
written exactly once when the verdict completes; a killed or crashed run leaves no
file).

## Screens & keys

- **Menu**: `↑/↓` select · `enter` confirm · `ctrl+c` quit.
- **New-debate form**: `tab`/`shift+tab` move between fields · `enter` next field (or
  submit from the last field) · `ctrl+s` submit · `esc` cancel.
- **Session view** (live or archived; exclusive full-screen, no menu chrome while a
  debate is open): `j/k` or `↑/↓` scroll the transcript · `a` abort the live debate
  (with confirmation; discards the in-memory transcript, nothing is archived) · `esc`
  back to the menu (a live debate keeps running in the background; reopen it from the
  archive list, marked `●`, to resume watching) · `ctrl+c` quit the app.
- **Archive list**: `↑/↓` select · `enter` open (read-only) · `esc` back to the menu.

Only one debate can be live at a time; starting a new one while another is running is
blocked until you finish or abort it.

## Debate modes

- **`proposition`** (default): one claim. `advocate` argues FOR, `critic` argues AGAINST.
- **`versus`**: two named options compete; each side advocates its own option.

## Tones

`formal` (default), `informal`, `genz`, `unhinged`. Tone is **style only**; it never lowers
the bar on substance, evidence, or honesty, and both sides share one tone so the debate stays
even.

## Evidence & code debates

Debaters back claims with **citations rendered as clickable markdown links** in the
transcript: `[label](https://…)` for external facts, `[path:line](path#Lline)` for code.
If the starting context contains a path that resolves to an existing directory, both
sides get sandboxed, read-only tool access (`read_file`, `grep`, `list_dir`) to that repo
so they can ground and cite real code; tool calls appear inline as collapsible `⚙` blocks
and are recorded in the archive alongside the messages.

## Layout

```
cmd/                    # entrypoint: bootstrap only, opens the TUI
internal/tui/           # bubbletea screens: menu, form, session view, archive list
internal/orchestrator/  # debate loop: roles, rounds, turn order, tool sub-loop, kronk adapter
internal/prompts/       # persona templates (advocate/critic/judge, modes, tones)
internal/tools/         # read_file, grep, list_dir (sandboxed to the detected repo)
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
  unit-tested against a fake `ChatClient`: no real model, no TTY.
- `internal/tui` and `features/` drive the actual Bubble Tea app via
  [`teatest`](https://github.com/charmbracelet/x/tree/main/exp/teatest) with a fake
  `ChatClient`, so the whole app (menu → form → live debate → verdict → archive →
  browse → abort) is exercised without ever loading a real model.
- An integration test against a real, tiny kronk model (e.g. `unsloth/Qwen3-0.6B-Q8_0`)
  is planned per `docs/SPEC.md` D12 but not required for everyday development; it would
  be gated behind `-short` or an opt-in environment variable so it never runs by default.
