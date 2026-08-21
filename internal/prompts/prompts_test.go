package prompts

import (
	"strings"
	"testing"
)

func TestDebaterSystem(t *testing.T) {
	tests := []struct {
		name           string
		params         DebaterParams
		wantSubstrings []string
	}{
		{
			name: "proposition leading argues FOR",
			params: DebaterParams{
				Mode: ModeProposition, Tone: ToneFormal, Label: "Advocate",
				OpponentLabel: "Critic", Leads: true, Rounds: 3,
			},
			wantSubstrings: []string{"argue FOR"},
		},
		{
			name: "proposition second argues AGAINST",
			params: DebaterParams{
				Mode: ModeProposition, Tone: ToneFormal, Label: "Critic",
				OpponentLabel: "Advocate", Leads: false, Rounds: 3,
			},
			wantSubstrings: []string{"argue AGAINST"},
		},
		{
			name: "versus uses stance labels",
			params: DebaterParams{
				Mode: ModeVersus, Tone: ToneFormal, Label: "Team Rust",
				Stance: "Rust", OpponentLabel: "Team Go", Leads: true, Rounds: 3,
			},
			wantSubstrings: []string{`"Rust"`, `"Team Go"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DebaterSystem(tt.params)
			for _, want := range tt.wantSubstrings {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in output, got: %s", want, got)
				}
			}
		})
	}
}

func TestToneGuidancePerTone(t *testing.T) {
	tests := []struct {
		name     string
		tone     Tone
		want     string
	}{
		{"formal", ToneFormal, "ALL CAPS for emphasis"},
		{"informal", ToneInformal, "chatroom register"},
		{"genz", ToneGenZ, "genz/genalpha slang"},
		{"unhinged", ToneUnhinged, "manic, chaotic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DebaterSystem(DebaterParams{Mode: ModeProposition, Tone: tt.tone, Leads: true, Rounds: 1})
			if !strings.Contains(p, tt.want) {
				t.Errorf("tone %q: expected persona to contain %q, got: %s", tt.tone, tt.want, p)
			}
		})
	}
}

func TestOpeningMessage(t *testing.T) {
	tests := []struct {
		name           string
		topic          string
		context        string
		round          int
		totalRounds    int
		wantSubstrings []string
	}{
		{
			name:           "bare topic no context",
			topic:          "Adopt X",
			context:        "",
			round:          1,
			totalRounds:    3,
			wantSubstrings: []string{"Adopt X", "plain prose only"},
		},
		{
			name:           "topic with context",
			topic:          "Adopt X",
			context:        "we use Y today",
			round:          1,
			totalRounds:    3,
			wantSubstrings: []string{"Adopt X", "we use Y today"},
		},
		{
			name:           "last round has final-turn note",
			topic:          "Adopt X",
			context:        "",
			round:          3,
			totalRounds:    3,
			wantSubstrings: []string{"last turn"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OpeningMessage(tt.topic, tt.context, tt.round, tt.totalRounds)
			for _, want := range tt.wantSubstrings {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in output, got: %s", want, got)
				}
			}
		})
	}
}

func TestPeerMessage(t *testing.T) {
	tests := []struct {
		name           string
		speaker        string
		content        string
		round          int
		totalRounds    int
		wantSubstrings []string
	}{
		{
			name:           "basic peer message",
			speaker:        "Critic",
			content:        "I disagree because Z",
			round:          2,
			totalRounds:    3,
			wantSubstrings: []string{"Critic", "I disagree because Z", "plain prose only"},
		},
		{
			name:           "last round has final-turn note",
			speaker:        "Critic",
			content:        "I disagree because Z",
			round:          3,
			totalRounds:    3,
			wantSubstrings: []string{"last turn"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PeerMessage(tt.speaker, tt.content, tt.round, tt.totalRounds)
			for _, want := range tt.wantSubstrings {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in output, got: %s", want, got)
				}
			}
		})
	}
}

func TestJudgeSystemDecisionFraming(t *testing.T) {
	tests := []struct {
		name           string
		mode           Mode
		wantSubstrings []string
	}{
		{"proposition", ModeProposition, []string{"adopt / reject"}},
		{"versus", ModeVersus, []string{"which option wins"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JudgeSystem(tt.mode)
			for _, want := range tt.wantSubstrings {
				if !strings.Contains(got, want) {
					t.Errorf("expected %q in output, got: %s", want, got)
				}
			}
		})
	}
}

func TestJudgeUserMessage(t *testing.T) {
	got := JudgeUserMessage("Adopt X", "advocate: ...\ncritic: ...")
	if !strings.Contains(got, "Adopt X") || !strings.Contains(got, "advocate: ...") {
		t.Errorf("expected judge message to include topic and transcript, got: %s", got)
	}
}
