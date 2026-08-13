package prompts

import (
	"strings"
	"testing"
)

func TestDebaterSystemPropositionLeadsForAgainst(t *testing.T) {
	advocate := DebaterSystem(DebaterParams{
		Mode: ModeProposition, Tone: ToneFormal, Label: "Advocate",
		OpponentLabel: "Critic", Leads: true, Rounds: 3,
	})
	if !strings.Contains(advocate, "argue FOR") {
		t.Errorf("expected leading side to argue FOR, got: %s", advocate)
	}

	critic := DebaterSystem(DebaterParams{
		Mode: ModeProposition, Tone: ToneFormal, Label: "Critic",
		OpponentLabel: "Advocate", Leads: false, Rounds: 3,
	})
	if !strings.Contains(critic, "argue AGAINST") {
		t.Errorf("expected second side to argue AGAINST, got: %s", critic)
	}
}

func TestDebaterSystemVersusUsesStance(t *testing.T) {
	p := DebaterSystem(DebaterParams{
		Mode: ModeVersus, Tone: ToneFormal, Label: "Team Rust",
		Stance: "Rust", OpponentLabel: "Team Go", Leads: true, Rounds: 3,
	})
	if !strings.Contains(p, `"Rust"`) || !strings.Contains(p, `"Team Go"`) {
		t.Errorf("expected versus persona to name stance and opponent, got: %s", p)
	}
}

func TestToneGuidancePerTone(t *testing.T) {
	cases := map[Tone]string{
		ToneFormal:   "Do not use ALL CAPS",
		ToneInformal: "chatroom register",
		ToneGenZ:     "genz/genalpha slang",
		ToneUnhinged: "manic, chaotic",
	}
	for tone, want := range cases {
		p := DebaterSystem(DebaterParams{Mode: ModeProposition, Tone: tone, Leads: true, Rounds: 1})
		if !strings.Contains(p, want) {
			t.Errorf("tone %q: expected persona to contain %q, got: %s", tone, want, p)
		}
	}
}

func TestOpeningMessage(t *testing.T) {
	got := OpeningMessage("Adopt X", "", 1, 3)
	if !strings.Contains(got, "Adopt X") {
		t.Errorf("expected bare topic when no context, got: %s", got)
	}
	if !strings.Contains(got, "plain prose only") {
		t.Errorf("expected turn note, got: %s", got)
	}
	got = OpeningMessage("Adopt X", "we use Y today", 1, 3)
	if !strings.Contains(got, "Adopt X") || !strings.Contains(got, "we use Y today") {
		t.Errorf("expected topic + context, got: %s", got)
	}
	if last := OpeningMessage("Adopt X", "", 3, 3); !strings.Contains(last, "last turn") {
		t.Errorf("expected final-turn note on the last round, got: %s", last)
	}
}

func TestPeerMessage(t *testing.T) {
	got := PeerMessage("Critic", "I disagree because Z", 2, 3)
	if !strings.Contains(got, "Critic") || !strings.Contains(got, "I disagree because Z") {
		t.Errorf("expected peer message to name speaker and content, got: %s", got)
	}
	if !strings.Contains(got, "plain prose only") {
		t.Errorf("expected turn note, got: %s", got)
	}
	if last := PeerMessage("Critic", "I disagree because Z", 3, 3); !strings.Contains(last, "last turn") {
		t.Errorf("expected final-turn note on the last round, got: %s", last)
	}
}

func TestJudgeSystemDecisionFraming(t *testing.T) {
	prop := JudgeSystem(ModeProposition)
	if !strings.Contains(prop, "adopt / reject") {
		t.Errorf("expected proposition judge framing, got: %s", prop)
	}
	versus := JudgeSystem(ModeVersus)
	if !strings.Contains(versus, "which option wins") {
		t.Errorf("expected versus judge framing, got: %s", versus)
	}
}

func TestJudgeUserMessage(t *testing.T) {
	got := JudgeUserMessage("Adopt X", "advocate: ...\ncritic: ...")
	if !strings.Contains(got, "Adopt X") || !strings.Contains(got, "advocate: ...") {
		t.Errorf("expected judge message to include topic and transcript, got: %s", got)
	}
}
