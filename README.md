# agon 
> or *agony* if you dont have enough VRAM.

Pit two AI agents against each other over any proposition and get a reasoned verdict.
The proposition can be a design/architecture choice, a technical tradeoff, a strategy or
policy, or a head-to-head X vs Y comparison.

The name comes from [Ancient Greek drama](https://en.wikipedia.org/wiki/Agon#:~:text=%5B9%5D-,Theater)

## Quick start

> ***NOTE***: This project is still in early stages of development.

```bash
go run ./agon
```

This opens the TUI at the main menu. 

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

## Evidence & debates

Debaters back claims with **citations rendered as clickable markdown links** in the
transcript: `[label](https://…)` for external facts, `[path:line](path#Lline)` for files.
If the starting context contains a path that resolves to an existing directory or file, both
sides get sandboxed, read-only tool access (`read_file`, `grep`, `list_dir`) to that location
so they can ground and cite real content; tool calls appear inline as collapsible `⚙` blocks
and are recorded in the archive alongside the messages.

## Makefile Targets

This project uses a Makefile with the following targets:
- `make build` - Compile the binary
- `make run` - Run the TUI in the foreground (Ctrl-C to quit)
- `make test` - Run all tests with the race detector, serialized (recommended for -race)
- `make vet` - Run go vet
- `make fmt` - Check gofmt formatting
- `make clean` - Remove the built binary
