// Package dialogue ports notebooks/01_dialogue.ipynb to a stateless GET endpoint.
//
// The web user plays Sam (the customer). Vera (economist) and Anton
// (philosopher) are LLM agents. Each call to GET /dialogue advances the
// conversation by one round — Sam's line, then Vera, then Anton — and returns
// the full updated transcript. The client sends the transcript back on the
// next call, so the Lambda itself stays stateless and no session store is
// needed.
//
// Abuse protection (a web user can send anything): Sam's line is length- and
// sentence-capped, the transcript is size-capped and strictly validated
// (speakers must alternate Sam → Vera → Anton; turn numbers and personas are
// recomputed server-side, never trusted from the client), and the number of
// rounds is capped.
package dialogue

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/lambdaapi"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/mistral"
)

const (
	// MaxSentences is the hard cap per utterance, as in the notebook (MAX_SENTENCES).
	MaxSentences = 5
	// maxTokensPerReply caps each model completion, as in the notebook (max_tokens=512).
	maxTokensPerReply = 512
	// maxMessageChars caps Sam's incoming line before sentence-splitting.
	maxMessageChars = 2000
	// maxEntryChars caps any single transcript line we accept from the client.
	maxEntryChars = 2000
	// defaultMaxTurns matches N_TURNS = 3 in the notebook.
	defaultMaxTurns = 3
	// hardMaxTurns is the absolute ceiling, even if DIALOGUE_MAX_TURNS asks for more.
	hardMaxTurns = 10
)

// Agent is one speaker in the dialogue. Sam is human (played by the web user).
type Agent struct {
	Name         string
	Persona      string
	SystemPrompt string
	Human        bool
}

// Sam is the web user; Vera and Anton are played by the model.
var (
	sam = Agent{Name: "Sam", Persona: "customer (you)", SystemPrompt: samPersona, Human: true}
	// modelAgents speak in turn order after Sam: Vera, then Anton.
	modelAgents = []Agent{
		{Name: "Vera", Persona: "economist", SystemPrompt: economistSystemPrompt},
		{Name: "Anton", Persona: "philosopher", SystemPrompt: philosopherSystemPrompt},
	}
)

// roundSpeakers is the required speaker order of every round: Sam → Vera → Anton.
var roundSpeakers = []string{sam.Name, modelAgents[0].Name, modelAgents[1].Name}

// Entry is one utterance in the transcript, as in the notebook.
type Entry struct {
	Turn    int    `json:"turn"`
	Speaker string `json:"speaker"`
	Persona string `json:"persona"`
	Content string `json:"content"`
}

// Response is the JSON body returned by GET /dialogue: the full transcript,
// including the new round.
type Response struct {
	Transcript []Entry `json:"transcript"`
}

// Handler returns the lambdaapi.HandlerFunc for GET /dialogue.
//
// Query parameters:
//
//	message     (required) Sam's new line for this round
//	transcript  (optional) base64url-encoded JSON []Entry from the previous
//	            response — echo it back to continue the conversation
func Handler(llm mistral.LLM) lambdaapi.HandlerFunc {
	return func(ctx context.Context, r *lambdaapi.Request) (any, error) {
		msg := strings.TrimSpace(r.Query.Get("message"))
		if msg == "" {
			return nil, lambdaapi.BadRequest("missing required query parameter: message (Sam's line)")
		}
		prior, err := decodeTranscript(r.Query.Get("transcript"))
		if err != nil {
			return nil, lambdaapi.BadRequest("invalid transcript parameter: %v", err)
		}
		transcript, err := NextRound(ctx, llm, msg, prior)
		if err != nil {
			return nil, err
		}
		return Response{Transcript: transcript}, nil
	}
}

// NextRound appends one full round (Sam → Vera → Anton) to prior and returns
// the complete transcript. It is a pure function of its inputs apart from the
// LLM calls, which makes it easy to test with a fake LLM.
func NextRound(ctx context.Context, llm mistral.LLM, samMessage string, prior []Entry) ([]Entry, error) {
	maxTurns := maxTurnsFromEnv()
	turn := len(prior)/len(roundSpeakers) + 1
	if turn > maxTurns {
		return nil, lambdaapi.BadRequest("the dialogue is over: at most %d turns are allowed", maxTurns)
	}

	// Sam's line: hard length cap, then the 5-sentence cap (as in the notebook).
	samMessage = LimitSentences(truncateChars(samMessage, maxMessageChars), MaxSentences)

	transcript := append([]Entry{}, prior...)
	transcript = append(transcript, Entry{Turn: turn, Speaker: sam.Name, Persona: sam.Persona, Content: samMessage})

	for _, agent := range modelAgents {
		reply, err := speak(ctx, llm, agent, transcript, turn)
		if err != nil {
			return nil, fmt.Errorf("agent %s: %w", agent.Name, err)
		}
		transcript = append(transcript, Entry{Turn: turn, Speaker: agent.Name, Persona: agent.Persona, Content: reply})
	}
	return transcript, nil
}

// speak is one model turn, mirroring speak() in the notebook: build messages
// from the labeled transcript, call the model, cap the reply at MaxSentences,
// strip a self-label if the model prefixes its own name.
func speak(ctx context.Context, llm mistral.LLM, agent Agent, transcript []Entry, turn int) (string, error) {
	reply, err := llm.Complete(ctx, BuildMessages(agent, transcript), maxTokensPerReply)
	if err != nil {
		return "", err
	}
	reply = LimitSentences(reply, MaxSentences)
	reply = selfLabel(agent.Name).ReplaceAllString(reply, "")
	return strings.TrimSpace(reply), nil
}

// BuildMessages mirrors build_messages() in the notebook: the speaker's system
// prompt, then the full labeled transcript (own lines as "assistant", others
// as "user"), then a final "your turn" user message.
func BuildMessages(speaker Agent, transcript []Entry) []mistral.Message {
	msgs := []mistral.Message{{Role: "system", Content: speaker.SystemPrompt}}
	for _, e := range transcript {
		role := "user"
		if e.Speaker == speaker.Name {
			role = "assistant"
		}
		msgs = append(msgs, mistral.Message{Role: role, Content: e.Speaker + ": " + e.Content})
	}
	msgs = append(msgs, mistral.Message{
		Role: "user",
		Content: fmt.Sprintf("The labeled transcript above is the full conversation so far. "+
			"Your turn, %s — reply in at most %d sentences.", speaker.Name, MaxSentences),
	})
	return msgs
}

// decodeTranscript parses and strictly validates the client-supplied transcript.
// Turn numbers and personas from the client are discarded and recomputed here:
// entries must follow the Sam → Vera → Anton round pattern, so a caller cannot
// forge Vera's or Anton's lines.
func decodeTranscript(encoded string) ([]Entry, error) {
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("not valid base64url: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	maxEntries := maxTurnsFromEnv() * len(roundSpeakers)
	if len(entries) > maxEntries {
		return nil, fmt.Errorf("transcript too long: at most %d entries (%d turns)", maxEntries, maxTurnsFromEnv())
	}
	if len(entries)%len(roundSpeakers) != 0 {
		return nil, fmt.Errorf("transcript has %d entries: incomplete round, must be a multiple of %d (Sam → Vera → Anton)", len(entries), len(roundSpeakers))
	}
	for i, e := range entries {
		want := roundSpeakers[i%len(roundSpeakers)]
		if e.Speaker != want {
			return nil, fmt.Errorf("entry %d must be spoken by %s, got %q", i, want, e.Speaker)
		}
		if strings.TrimSpace(e.Content) == "" {
			return nil, fmt.Errorf("entry %d has empty content", i)
		}
		if len([]rune(e.Content)) > maxEntryChars {
			return nil, fmt.Errorf("entry %d too long (max %d chars)", i, maxEntryChars)
		}
		// Recompute server-side fields; never trust the client's copy.
		entries[i].Turn = i/len(roundSpeakers) + 1
		entries[i].Persona = personaOf(e.Speaker)
	}
	return entries, nil
}

func personaOf(name string) string {
	if name == sam.Name {
		return sam.Persona
	}
	for _, a := range modelAgents {
		if a.Name == name {
			return a.Persona
		}
	}
	return ""
}

// maxTurnsFromEnv reads DIALOGUE_MAX_TURNS (default 3, as N_TURNS in the
// notebook), clamped to [1, hardMaxTurns] so the env var cannot be abused to
// force unbounded LLM spending.
func maxTurnsFromEnv() int {
	n := defaultMaxTurns
	if v := os.Getenv("DIALOGUE_MAX_TURNS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed
		}
	}
	if n < 1 {
		n = 1
	}
	if n > hardMaxTurns {
		n = hardMaxTurns
	}
	return n
}

func truncateChars(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// SplitSentences splits text into sentences on '.', '!' or '?' followed by
// whitespace or end of text. (Go's regexp has no lookbehind, so we scan.)
func SplitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '.', '!', '?':
			if i+1 == len(text) || text[i+1] == ' ' || text[i+1] == '\t' || text[i+1] == '\n' {
				if s := strings.TrimSpace(text[start : i+1]); s != "" {
					out = append(out, s)
				}
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

// LimitSentences keeps at most max sentences — the notebook's safety net.
func LimitSentences(text string, max int) string {
	s := SplitSentences(text)
	if len(s) > max {
		s = s[:max]
	}
	return strings.Join(s, " ")
}

// selfLabel matches a leading "Vera: " style prefix the model might add.
func selfLabel(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `\s*:\s*`)
}
