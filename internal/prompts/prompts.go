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

// Tone is the debaters' shared language register.
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

// Tone guidance blocks
const (
	ToneFormalGuidance = `TONE: formal: formal and factual; back every claim with evidence. Do
		"not use ALL CAPS for emphasis — let the explanation carry its own weight.`

	ToneInformalGuidance = `TONE: informal: a relaxed, chatroom register; some looseness over 
		"formal is fine. Stay respectful and keep the argument substantive.`

	ToneGenZGuidance = `TONE: genz: genz/genalpha slang and imageboard lingo, while staying
		"respectful and non-offensive. Substance and evidence are still required.`

	ToneUnhingedGuidance = `TONE: unhinged: manic, chaotic, over-the-top energy: caps for emphasis,
		wild metaphors, and theatrical conviction are all fair game. Stay respectful
		and non-offensive — it's the delivery that's feral, not the content.
		Substance and evidence are still required; every claim still gets backed.`
)

// toneGuidance renders the "TONE" block for a single register.
func toneGuidance(t Tone) string {
	switch t {
	case ToneInformal:
		return ToneInformalGuidance
	case ToneGenZ:
		return ToneGenZGuidance
	case ToneUnhinged:
		return ToneUnhingedGuidance
	default:
		return ToneFormalGuidance
	}
}

const EvidenceGuidancePrompt = `
EVIDENCE — back every nontrivial claim with a citation, written as a markdown link:
  - external facts/benchmarks/docs: [short label](https://real-url)")
  - For code claims: cite the actual file you read, as [path/to/file.ext:42](path/to/file.ext#L42)
	- For code, ACTUALLY READ the file in the repo (read_file/grep/list_dir) before citing it. 
	  Never invent paths, symbols, line numbers, or quotes. If you can't verify it in the sandbox, 
	  don't assert it — say what you'd need to check instead.	
  - For local files, READ the file, then include a snippet of the data you are using for evidence. 
    If the data is non human readable, do not include the raw data in the output, put point out where
	in the file you are refering to.
`

// Role line templates for the opening "who you are" sentence. Each takes
// fmt verbs filled in by DebaterSystem.
const (
	RoleVersusFormat    = "You are arguing the position %q in a debate against %q."
	RoleProponentFormat = "You are %s. You argue FOR the proposition under debate."
	RoleOpponentFormat  = "You are %s. You argue AGAINST the proposition under debate."
)

// Stance blocks for how each side is told to argue.
const (
	StanceVersus = `Make the strongest HONEST case for your option: concrete benefits and 
		evidence (with citations), and how you'd answer the other side's objections. 
		Attack the other option's real weaknesses: shortcomings, risks, hidden costs, 
		failure modes, without strawmanning it.`

	StanceProponent = `Make the strongest HONEST case for the proposition: concrete benefits and
		evidence (with citations), and how you'd answer objections. Do not strawman 
		the opposing view.`

	StanceOpponent = `Attack the proposition's real weaknesses: shortcomings, risks, hidden 
		costs, failure modes, and concede a point when it genuinely holds. Attack 
		real weaknesses, not strawmen.`
)

// Turn-orders, for whether the side moves first or second each round.
const (
	OrderLeads = `You move FIRST each round: open with your case (round 1) or your 
		rebuttal (later rounds), then wait for your opponent's reply.`
	OrderFollows = `You move SECOND each round: read your opponent's message, then rebut it 
		and add your own case or new objections.`
)

// TurnRulesPrompt is the shared "write one plain-prose turn" rulebook.
const TurnRulesPrompt = `This is a live, turn-by-turn exchange: you are called back separately for every turn you take, and you only ever see the transcript so far — never the whole debate in advance, and never your opponent's future replies. Write ONLY your single next turn, as plain continuous prose (try to keep within a few 
	paragraphs ). Do not, under any circumstances:
  - invent, number, preview, or summarize other turns/rounds (no "Round 1:", "Round 2:", "Final round:", "Final answer:", or similar labels/headers)
  - write your opponent's reply, or narrate what they will say
  - use markdown section headers (#, ##) to structure your reply into stages
If you catch yourself writing a label like the ones above, stop — delete it and just continue the argument in plain prose instead.`

// ToolsPrompt describes the read-only tool sandbox available to debaters.
const ToolsPrompt = `You have read-only tools (read_file, grep, list_dir) scoped to 
	"the directories under debate.
	If the starting context has not listed any paths on the system, or there were no detected paths, do not 
	run a tool call to read from anything. Only use the read_file, list_dir, and grep tools when certain the user gave a path to analyze. 
	`

// SandboxDirsIntro precedes the bulleted list of sandbox directories.
const SandboxDirsIntro = `Sandbox directories (tool paths may be absolute within these, 
	"or relative to them):`

// Per-turn reminders appended to user messages. They intentionally avoid
// "Round N of M"-style bracketed framing as smaller models seems to thing they need to
// include multiple rounds in one statement if seen in the system prompt.
const (
	TurnNoteFinal = "(This is your last turn in the debate make it count, but still just " +
		"one plain-prose reply, no headers or labels.)"
	TurnNoteNormal = "(Reply with plain prose only, no headers, no labels, no other turns.)"
)

// User-message assembly bits.
const (
	OpeningContextLabel    = "\n\nContext:\n"
	PeerMessageFormat      = "%s said:\n\n%s\n\n%s"
	JudgeUserMessageFormat = "Proposition:\n%s\n\nFull transcript:\n%s\n\nWrite your verdict."
)

// Judge persona pieces.
const (
	JudgeDecisionVersus = `Decide which option wins, and under what conditions the other option
		"would instead be the right call.`

	JudgeDecisionProposition = `Decide: adopt / reject / adopt-with-modifications for a proposal, or
		 true / false / partly-true for a factual claim.`

	JudgeSystemPreamble = `You are a neutral adjudicator. You did not argue either side. Judge on the 
		 merits, on substance not style. The debaters' tone does not affect who is right. 
		 Weigh cited, verifiable claims above unsupported assertions, and discount any 
		 citation that misrepresents its source.`

	JudgeSystemStructure = `Write your verdict in a neutral, formal register regardless of the debate's tone.
		Structure it as:
		- Decision: be definite.
		- Rationale: the reasoning, naming the specific arguments and citations that 
		  drove it.
		- Key risks / conditions — the surviving objections; what must hold or be 
		  mitigated, or the conditions under which the losing side would instead be right.`
)

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

// DebaterSystem renders the persistent system persona for one debater depending on the
// debate parameters.
func DebaterSystem(p DebaterParams) string {
	var role string
	switch {
	case p.Mode == ModeVersus:
		role = fmt.Sprintf(RoleVersusFormat, p.Stance, p.OpponentLabel)
	case p.Leads:
		role = fmt.Sprintf(RoleProponentFormat, p.Label)
	default:
		role = fmt.Sprintf(RoleOpponentFormat, p.Label)
	}

	var stance string
	switch {
	case p.Mode == ModeVersus:
		stance = StanceVersus
	case p.Leads:
		stance = StanceProponent
	default:
		stance = StanceOpponent
	}

	order := OrderFollows
	if p.Leads {
		order = OrderLeads
	}

	out := strings.Join([]string{
		role,
		stance,
		order,
		TurnRulesPrompt,
		toneGuidance(p.Tone),
		ToolsPrompt,
	}, "\n\n")

	if len(p.Dirs) > 0 {
		out += "\n" + SandboxDirsIntro
		for _, d := range p.Dirs {
			out += "\n  - " + d
		}
	}
	return out
}

// turnNote appends a short, non-header reminder to a per-turn user message.
func turnNote(final bool) string {
	if final {
		return TurnNoteFinal
	}
	return TurnNoteNormal
}

// OpeningMessage renders the first user message sent to the leading side: the
// full proposition plus its starting context.
func OpeningMessage(topic, startingContext string, round, rounds int) string {
	msg := topic
	if strings.TrimSpace(startingContext) != "" {
		msg = topic + OpeningContextLabel + startingContext
	}
	return msg + "\n\n" + turnNote(round == rounds)
}

// PeerMessage renders the user message a side receives to respond to its
// opponent's last turn.
func PeerMessage(opponentLabel, content string, round, rounds int) string {
	return fmt.Sprintf(PeerMessageFormat, opponentLabel, content, turnNote(round == rounds))
}

// JudgeSystem renders the neutral judge's persistent system persona.
func JudgeSystem(mode Mode) string {
	decision := JudgeDecisionProposition
	if mode == ModeVersus {
		decision = JudgeDecisionVersus
	}
	return JudgeSystemPreamble + decision + "\n\n" + JudgeSystemStructure
}

// JudgeUserMessage renders the single user message the judge receives: the
// full transcript, verbatim.
func JudgeUserMessage(topic, transcript string) string {
	return fmt.Sprintf(JudgeUserMessageFormat, topic, transcript)
}
