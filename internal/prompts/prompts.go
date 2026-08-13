// Package prompts holds the debate personas and instructions as Go string
// literals, ported from the retired .claude/skills/debate/SKILL.md. There is
// no template file on disk: every prompt is composed in code from the mode,
// tone, and side in play.
package prompts

import (
	"fmt"
	"strings"
)

// Mode selects the shape of a debate.
type Mode string

const (
	// ModeProposition pits one claim under test: advocate argues FOR,
	// critic argues AGAINST.
	ModeProposition Mode = "proposition"
	// ModeVersus pits two named options against each other; each side
	// advocates its own option.
	ModeVersus Mode = "versus"
)

// Tone is the debaters' shared language register. It governs style only —
// it never lowers the bar on substance, evidence, or honesty.
type Tone string

const (
	ToneFormal   Tone = "formal"
	ToneInformal Tone = "informal"
	ToneGenZ     Tone = "genz"
	ToneUnhinged Tone = "unhinged"
)

// ValidModes lists every accepted Mode.
var ValidModes = []Mode{ModeProposition, ModeVersus}

// ValidTones lists every accepted Tone.
var ValidTones = []Tone{ToneFormal, ToneInformal, ToneGenZ, ToneUnhinged}

// DefaultTone is used when a caller doesn't specify one.
const DefaultTone = ToneFormal

// DefaultMode is used when a caller doesn't specify one.
const DefaultMode = ModeProposition

// DefaultRounds is the pre-filled round count in the new-debate form.
const DefaultRounds = 3

// toneGuidance renders the SKILL.md "TONE" block for a single register.
func toneGuidance(t Tone) string {
	switch t {
	case ToneInformal:
		return "TONE — informal: a relaxed, chatroom register; some looseness over " +
			"formal is fine. Stay respectful and keep the argument substantive."
	case ToneGenZ:
		return "TONE — genz: genz/genalpha slang and imageboard lingo, while staying " +
			"respectful and non-offensive. Substance and evidence are still required."
	case ToneUnhinged:
		return "TONE — unhinged: manic, chaotic, over-the-top energy: caps for emphasis, " +
			"wild metaphors, and theatrical conviction are all fair game. Stay respectful " +
			"and non-offensive — it's the delivery that's feral, not the content. " +
			"Substance and evidence are still required; every claim still gets backed."
	default:
		return "TONE — formal: formal and factual; back every claim with evidence. Do " +
			"not use ALL CAPS for emphasis — let the explanation carry its own weight."
	}
}

// evidenceGuidance renders the SKILL.md "EVIDENCE" block.
func evidenceGuidance(hasRepo bool) string {
	var b strings.Builder
	b.WriteString("EVIDENCE — back every nontrivial claim with a citation, written as a " +
		"markdown link:\n")
	b.WriteString("  - external facts/benchmarks/docs: [short label](https://real-url)\n")
	if hasRepo {
		b.WriteString("  - code claims: cite the actual file you read, as " +
			"[path/to/file.ext:42](path/to/file.ext#L42)\n")
		b.WriteString("Rules:\n" +
			"  - For code, ACTUALLY READ the file in the repo (read_file/grep/list_dir) " +
			"before citing it. Never invent paths, symbols, line numbers, or quotes. If you " +
			"can't verify it in the repo, don't assert it — say what you'd need to check " +
			"instead.\n")
	}
	b.WriteString("  - Prefer primary sources. One precise, verifiable citation beats " +
		"three vague ones. Use available assets within your confined sandbox, such as code, documents, etc.\n")
	b.WriteString("  - Unsupported assertions are fair to make but count for less; the " +
		"judge weighs cited claims more heavily, so cite your strongest points.")
	return b.String()
}

// DebaterParams configures a single debater's persistent system persona.
type DebaterParams struct {
	Mode          Mode
	Tone          Tone
	Label         string // human label, e.g. "Advocate" or "Team Rust"
	Stance        string // the position this side argues
	OpponentLabel string
	Leads         bool // true for the side that moves first each round
	Rounds        int
	Dirs          []string // sandbox directories tools are confined to
}

// DebaterSystem renders the persistent system persona for one debater.
func DebaterSystem(p DebaterParams) string {
	var role string
	switch {
	case p.Mode == ModeVersus:
		role = fmt.Sprintf("You are arguing the position %q in a debate against %q.",
			p.Stance, p.OpponentLabel)
	case p.Leads:
		role = fmt.Sprintf("You are %s. You argue FOR the proposition under debate.", p.Label)
	default:
		role = fmt.Sprintf("You are %s. You argue AGAINST the proposition under debate.", p.Label)
	}

	var stance string
	switch {
	case p.Mode == ModeVersus:
		stance = "Make the strongest HONEST case for your option: concrete benefits and " +
			"evidence (with citations), and how you'd answer the other side's objections. " +
			"Attack the other option's real weaknesses — shortcomings, risks, hidden costs, " +
			"failure modes — without strawmanning it."
	case p.Leads:
		stance = "Make the strongest HONEST case for the proposition: concrete benefits and " +
			"evidence (with citations), and how you'd answer objections. Do not strawman " +
			"the opposing view."
	default:
		stance = "Attack the proposition's real weaknesses — shortcomings, risks, hidden " +
			"costs, failure modes — and concede a point when it genuinely holds. Attack " +
			"real weaknesses, not strawmen."
	}

	var order string
	if p.Leads {
		order = "You move FIRST each round: open with your case (round 1) or your " +
			"rebuttal (later rounds), then wait for your opponent's reply."
	} else {
		order = "You move SECOND each round: read your opponent's message, then rebut it " +
			"and add your own case or new objections."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s\n\n%s\n\n", role, stance, order)
	b.WriteString("This is a live, turn-by-turn exchange: you are called back separately for " +
		"every turn you take, and you only ever see the transcript so far — never the whole " +
		"debate in advance, and never your opponent's future replies. Write ONLY your single " +
		"next turn, as plain continuous prose (a few tight paragraphs). Do not, under any " +
		"circumstances:\n" +
		"  - invent, number, preview, or summarize other turns/rounds (no \"Round 1:\", " +
		"\"Round 2:\", \"Final round:\", \"Final answer:\", or similar labels/headers)\n" +
		"  - write your opponent's reply, or narrate what they will say\n" +
		"  - use markdown section headers (#, ##) to structure your reply into stages\n" +
		"If you catch yourself writing a label like the ones above, stop — delete it and " +
		"just continue the argument in plain prose instead.\n\n")
	b.WriteString(toneGuidance(p.Tone))
	b.WriteString("\n\n")
	b.WriteString("\n\nYou have read-only tools (read_file, grep, list_dir) scoped to " +
		"the directories under debate. Use them to verify and cite real code — never to " +
		"modify anything.")
	if len(p.Dirs) > 0 {
		b.WriteString("\nSandbox directories (tool paths may be absolute within these, " +
			"or relative to them):")
		for _, d := range p.Dirs {
			b.WriteString("\n  - " + d)
		}
	}
	return b.String()
}

// turnNote appends a short, non-header reminder to a per-turn user message.
// It intentionally avoids "Round N of M"-style bracketed framing: earlier we
// found that literally spelling out round numbers in the prompt primed
// weaker models to echo that same framing back as headers in their own
// reply (e.g. writing "Round 2: ..." then drafting ahead into "Round 3: ...")
// instead of writing one plain-prose turn.
func turnNote(final bool) string {
	if final {
		return "(This is your last turn in the debate — make it count, but still just " +
			"one plain-prose reply, no headers or labels.)"
	}
	return "(Reply with plain prose only — no headers, no labels, no other turns.)"
}

// OpeningMessage renders the first user message sent to the leading side: the
// full proposition plus its starting context.
func OpeningMessage(topic, startingContext string, round, rounds int) string {
	msg := topic
	if strings.TrimSpace(startingContext) != "" {
		msg = topic + "\n\nContext:\n" + startingContext
	}
	return msg + "\n\n" + turnNote(round == rounds)
}

// PeerMessage renders the user message a side receives to respond to its
// opponent's last turn.
func PeerMessage(opponentLabel, content string, round, rounds int) string {
	return fmt.Sprintf("%s said:\n\n%s\n\n%s", opponentLabel, content, turnNote(round == rounds))
}

// JudgeSystem renders the neutral judge's persistent system persona.
func JudgeSystem(mode Mode) string {
	var decision string
	if mode == ModeVersus {
		decision = "Decide which option wins, and under what conditions the other option " +
			"would instead be the right call."
	} else {
		decision = "Decide: adopt / reject / adopt-with-modifications for a proposal, or " +
			"true / false / partly-true for a factual claim."
	}

	return "You are a neutral adjudicator. You did not argue either side. Judge on the " +
		"merits, on substance not style — the debaters' tone does not affect who is right. " +
		"Weigh cited, verifiable claims above unsupported assertions, and discount any " +
		"citation that misrepresents its source. " + decision + "\n\n" +
		"Write your verdict in a neutral, formal register regardless of the debate's tone. " +
		"Structure it as:\n" +
		"- Decision — be definite.\n" +
		"- Rationale — the reasoning, naming the specific arguments and citations that " +
		"drove it.\n" +
		"- Key risks / conditions — the surviving objections; what must hold or be " +
		"mitigated, or the conditions under which the losing side would instead be right."
}

// JudgeUserMessage renders the single user message the judge receives: the
// full transcript, verbatim.
func JudgeUserMessage(topic, transcript string) string {
	return fmt.Sprintf("Proposition:\n%s\n\nFull transcript:\n%s\n\nWrite your verdict.",
		topic, transcript)
}
