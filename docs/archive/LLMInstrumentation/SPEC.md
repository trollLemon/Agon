# goagentdisc TUI — Spec

## Goal

Replace the harness-driven MCP debate server with a standalone TUI application
for running two-agent debates on **local models** via the
[kronk](https://github.com/ardanlabs/kronk) SDK, in-process. No Claude Code /
Copilot CLI harness, no long-running server, no MCP.

Architecture and process diagrams: `docs/diagrams/goagentdisc-architecture.drawio`
(pages: 1 system architecture, 2 startup/provisioning, 3 orchestration loop,
4 TUI navigation, 5 archive data model; PNG renders alongside).

## Decisions

### D1 — Model access: in-process kronk SDK

- Single self-contained binary; loads model(s) on demand, exits when done.
- No kronk model server, no OpenAI-compatible HTTP client layer.
- Removes: `internal/server` (MCP/HTTP/SSE/webapp), `.mcp.json`,
  `.claude/skills/debate/SKILL.md` as a runtime mechanism (content ported
  into Go — see D10).

### D2 — Model assignment: one model, per-role kronk sessions

- One loaded model instance plays advocate, critic, and judge.
- Each pseudo-agent = a persistent message history (system persona + one
  user/assistant pair per turn) sent via `krn.ChatStreaming`; kronk's
  incremental message cache (IMC, `model.WithIncrementalCache(true)`)
  matches the prefix and reuses KV automatically — sessions externalize
  their KV via the session store, so no full-transcript re-processing.
  (kronk IMC session IDs are internal pool identities; the app does not
  pass session IDs in chat docs.)
  - `advocate`: system = FOR persona (+ tone/mode); topic + starting
    context first, then opponent messages.
  - `critic`: system = AGAINST persona; same pattern.
  - `judge`: system = neutral adjudicator; receives full transcript once.
- Orchestration is a synchronous loop inside the app; the MCP `post`/`wait`
  slot protocol disappears.
- Known tradeoff (deferred from MVP): every session carries its own KV
  context, so memory scales with the number of pseudo-agents (3). A smaller
  model for the judge (and any future orchestrator chatter) is a recognized
  post-MVP optimization — the per-role model override point should be kept
  in mind in the config design, but not built.

### D3 — Interactivity: spectate-only v1

- Start debate → watch streamed transcript → verdict → exit/archive.
- Orchestrator is a synchronous pipeline, not an event loop; no pause/steer
  in v1. (Design the loop so turn boundaries are clean extension points
  later.)

### D4 — TUI stack: Bubble Tea

- `bubbletea` (Elm-style update loop) + `bubbles/viewport` for scrollback
  + `glamour` for rendering debater markdown (citations/links) inline.
- Streaming tokens map onto bubbletea messages (one msg per token/chunk).

### D5 — Archive format: one JSON file per session, written once

- `debates/<session_id>.json` (flat dir, not per-session subdirs).
- Session IDs keep the current timestamp-based naming scheme
  (`titleFromDirname` logic carries over to filenames).
- `rounds` defaults to 3 (form field pre-fills 3).
- **Write-once semantics**: the transcript lives in memory during the
  debate; the JSON is written exactly once, atomically (tmp + rename),
  when the verdict completes. Full transcript or none at all — a killed
  or crashed run leaves no file, and there is no `incomplete` status.
  (Accepted MVP tradeoff: a crash after a long debate loses everything;
  may be revisited post-MVP.)
- Archive browsing = read one file; no legacy loader for the old
  `transcript/*.md` + `verdict.md` layout (POC clean break).
- Schema (see also diagram page 5):

```json
{
  "session_id": "2026-08-12-…",
  "title": "…",
  "topic": "…",
  "mode": "proposition | versus",
  "tone": "formal | informal | genz | unhinged",
  "rounds": 3,
  "sides": [{"id": "advocate", "label": "FOR", "stance": "…"}],
  "model": "unsloth/Qwen3-…",
  "dirs": ["sandbox directories detected from context text, or absent"],
  "created_at": "2026-08-12T19:00:00Z",
  "messages": [
    {
      "role": "advocate",
      "round": 1,
      "content": "…markdown…",
      "tool_calls": [{"name": "read_file", "args": "…", "result_summary": "…"}],
      "ts": 1234.5
    }
  ],
  "verdict": "…markdown…",
  "aborted": {}
}
```

### D6 — Repo grounding: keep, with visible tool calls; repo comes from the input text

- Debate start input = **topic + freeform starting context** — the
  equivalent of the old harness flow where you pasted a prompt mentioning
  a repo and what you want changed. No dedicated repo flag/field.
- The app scans the starting context for paths that resolve to existing
  directories; if any are found, the read-only tools are enabled and
  sandboxed to those directories (git repos or not — any directory counts).
  No valid path → prompt-only debate, tools off.
- Read-only tool set: `read_file`, `grep`, `list_dir`.
- kronk tool-call loop executed in-app; max ~8 tool iterations per turn to
  bound small-model looping.
- Tool calls are first-class UI events: streamed to the TUI as inline
  collapsible blocks (⚙ tool name + args + result size) and recorded in the
  session archive alongside messages, so citations stay auditable.
- Debaters keep the current citation convention: `[path:line](path#Lline)`.

### D7 — Model management: defer to kronk SDK managers; no CLI flags

- Minimal CLI: running the binary opens the TUI, period. No debate flags.
- Reuse kronk's `sdk/tools/libs` + `sdk/tools/models` managers exactly as
  the examples do: first run auto-downloads compatible native libs and the
  model (progress via `kronk.FmtLogger`); cached thereafter.
- Model source is a field in the new-debate form, pre-filled with a
  built-in default constant (HF URI or `provider/modelID`, e.g.
  `unsloth/Qwen3-0.6B-Q8_0` from the examples). No catalog/wizard for MVP.
- Reference implementations in the kronk repo:
  - `examples/question` — lib/model install + basic chat
  - `examples/session-store` — IMC + session store factory
  - `examples/agent` — tool-call loop + read-only tools to crib
    (`RegisterReadFile`, `RegisterSearchFiles`)

### D8 — Navigation & screens

- Running the binary always opens the TUI at the **menu** (new · archive ·
  resume live) — the only entry point (no CLI bypass; the form is reached
  from the menu).
- Form fields: topic, starting context (freeform multiline — paste repo
  path / change description here), mode, tone, rounds (def 3), model.
- Screens: (0) menu, (1) new-debate form, (2) session view,
  (3) archive list.
- **Session view is exclusive**: while viewing a debate (live or archived)
  it is the only thing rendered — header (title, round x/N, tone, ●/◾),
  full-height scrolling transcript (glamour-rendered markdown, inline ⚙
  tool-call blocks), verdict banner when done. No menu/sidebar chrome.
- `esc` backs out to the menu; the archive list and form are only rendered
  when no session is being viewed. Archived debates open in the same
  session view, read-only.
- Backing out of a **live** debate leaves it running in background;
  archive list marks it ● live and re-entering resumes the stream.
- Live debates run on their own goroutine feeding the TUI via bubbletea
  messages (token chunks, tool calls, turn boundaries, verdict, errors);
  the archive JSON is written once when the debate completes (D5),
  regardless of which screen is focused.
- Keys in session view: `j/k` scroll · `a` abort (confirm, live only) ·
  `esc` back out · `ctrl+c` quit app.

### D9 — One debate at a time

- Starting a new debate while one is live is blocked in the menu
  (finish or abort the live one first). KV memory stays bounded.

### D10 — Personas/prompts: Go string literals ported from SKILL.md

- `.claude/skills/debate/SKILL.md` content (mode rules, tone definitions,
  evidence/citation standards, judge instructions) is ported into
  `internal/prompts` as **raw-string literals in Go code** — no embed.FS,
  no template files on disk. Rendered with mode/tone/side vars
  (`text/template` or simple sprintf-style substitution).
- The skill file and `.mcp.json` are deleted — no harness reads them
  anymore.

### D11 — Orchestration loop & error handling

- Turn order (unchanged from current design): advocate opens each round,
  critic responds; after N rounds the judge receives the full transcript
  once and produces the verdict.
- Each turn: append one user message to the role's history →
  `ChatStreaming` → tool-call sub-loop (D6) → final text → next turn.
  Tokens stream to the TUI as they arrive. Transcript accumulates in
  memory; nothing touches disk until completion.
- On verdict: single atomic archive write (D5).
- Errors: a failed turn marks the session errored, shows an error banner
  in the session view; nothing is archived. User can back out and start
  fresh.
- `ctrl+c` exits the app; the in-flight debate dies with the process and
  leaves no archive file.
- Explicit abort: `a` in live session view (with confirm) stops the
  goroutine and discards the in-memory transcript (full transcript or
  none at all).

### D12 — Testing: unit + integration + BDD

- **Unit** (`go test`, no model, no TTY):
  - `internal/orchestrator` — turn order, tool sub-loop (max-8 bound),
    abort/error paths, against a fake model client (interface-injected
    `ChatStreaming` stub).
  - `internal/archive` — write-once atomic write (tmp+rename), schema
    round-trip, list/load.
  - `internal/prompts` — persona rendering per mode/tone/side.
  - repo-path detection from starting-context text (valid dir → tools on;
    none/garbage → tools off).
  - `internal/tools` — sandboxing (path escape attempts rejected).
- **Integration** (real in-process kronk, tiny model e.g. Qwen3-0.6B):
  end-to-end 1-round debate → verdict → archive file exists and parses;
  tool call made against a fixture repo appears in the transcript.
  Skippable via `-short` for machines without the model/libs.
- **BDD** (godog feature files under `features/`): user-facing flows —
  open app at menu, start debate from form, watch streamed turns, verdict
  + archive written, browse archive, open archived debate read-only,
  abort discards. TUI driven via `teatest` (charmbracelet's test harness).
- Existing `internal/debate` + `internal/server` tests are deleted with
  the server code they cover.

## Target package layout

```
cmd/        # entrypoint: bootstrap only (no debate flags)
internal/tui/           # bubbletea screens: menu, form, session view, archive list
internal/orchestrator/  # debate loop: roles, rounds, turn order, tool sub-loop
internal/prompts/       # persona templates as Go raw strings (from SKILL.md)
internal/tools/         # read_file, grep, list_dir — sandboxed to detected dirs
internal/archive/       # one-JSON-per-session: write-once, list, load
features/               # godog BDD feature files
docs/diagrams/          # goagentdisc-architecture.drawio + PNG renders
```

## Removed vs the current design

- `internal/server` (MCP tools, HTTP, SSE, embedded webapp) — gone
- `.mcp.json`, `.claude/skills/debate/SKILL.md` — gone (content ported to
  `internal/prompts`)
- `post`/`wait` slot protocol, `DEBATE_*` env vars — gone

## Kept from the current design

- Debate domain concepts: modes (proposition/versus), tones, rounds, sides
- Evidence/citation standards (`[path:line](path#Lline)`, markdown links)
- Verdict flow with a neutral judge
- Archive browsing of past debates
- Timestamp-based session ID naming


## Coding guidelines:
- Keep comments minimal unless code is not trivial. I.e dont write a comment after each line if the line is os.Open.
- ALWAYS document public exported functions.
- Do not add interfaces and indirection unless absolutely necessary. 
- Do not add comments mentioning a behavior is done to mimic the old MCP server design, those are not necessary.
- Ensure architecture is easy to extend to avoid tech debt.
