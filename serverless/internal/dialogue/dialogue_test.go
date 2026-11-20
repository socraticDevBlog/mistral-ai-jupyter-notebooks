package dialogue

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/lambdaapi"
	"github.com/socraticDevBlog/mistral-ai-jupyter-notebooks/serverless/internal/mistral"
)

// fakeLLM records the messages it was called with and replies per agent name.
type fakeLLM struct {
	calls   [][]mistral.Message
	reply   string
	wantErr bool
}

func (f *fakeLLM) Complete(ctx context.Context, msgs []mistral.Message, maxTokens int) (string, error) {
	f.calls = append(f.calls, msgs)
	if f.wantErr {
		return "", errors.New("llm down")
	}
	return f.reply, nil
}

func encode(t *testing.T, entries []Entry) string {
	t.Helper()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestNextRoundRunsSamVeraAnton(t *testing.T) {
	llm := &fakeLLM{reply: "Interesting point about incentives."}
	tr, err := NextRound(context.Background(), llm, "What will AI do to jobs?", nil)
	if err != nil {
		t.Fatalf("NextRound: %v", err)
	}
	if len(tr) != 3 {
		t.Fatalf("transcript has %d entries, want 3 (Sam, Vera, Anton)", len(tr))
	}
	wantSpeakers := []string{"Sam", "Vera", "Anton"}
	for i, e := range tr {
		if e.Speaker != wantSpeakers[i] || e.Turn != 1 {
			t.Errorf("entry %d = %+v, want speaker %s turn 1", i, e, wantSpeakers[i])
		}
	}
	if tr[0].Content != "What will AI do to jobs?" {
		t.Errorf("Sam's entry = %q", tr[0].Content)
	}
	// Two model calls: Vera then Anton.
	if len(llm.calls) != 2 {
		t.Fatalf("llm called %d times, want 2", len(llm.calls))
	}
}

func TestBuildMessagesLabeledTranscriptAndRoles(t *testing.T) {
	llm := &fakeLLM{reply: "Vera says hi."}
	tr, err := NextRound(context.Background(), llm, "opening line", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Build the messages Anton would see: system prompt + Sam & Vera lines + "your turn".
	msgs := BuildMessages(modelAgents[1], tr[:2]) // Sam + Vera only
	if msgs[0].Role != "system" || msgs[0].Content != philosopherSystemPrompt {
		t.Errorf("first message is not Anton's system prompt")
	}
	if msgs[1].Role != "user" || msgs[1].Content != "Sam: opening line" {
		t.Errorf("Sam's line wrong: %+v", msgs[1])
	}
	if msgs[2].Role != "user" || msgs[2].Content != "Vera: Vera says hi." {
		t.Errorf("Vera's line wrong: %+v", msgs[2])
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" || !strings.Contains(last.Content, "Your turn, Anton") {
		t.Errorf("final prompt wrong: %+v", last)
	}

	// Vera must see her OWN line as assistant and Sam's as user.
	veraMsgs := BuildMessages(modelAgents[0], tr)
	if veraMsgs[1].Role != "user" || veraMsgs[2].Role != "assistant" {
		t.Errorf("roles for Vera wrong: %+v", veraMsgs[1:3])
	}
}

func TestSamLineIsSentenceCapped(t *testing.T) {
	llm := &fakeLLM{reply: "ok."}
	long := strings.Join([]string{"One.", "Two.", "Three.", "Four.", "Five.", "Six.", "Seven."}, " ")
	tr, err := NextRound(context.Background(), llm, long, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(SplitSentences(tr[0].Content)); got != MaxSentences {
		t.Errorf("Sam's line has %d sentences, want %d", got, MaxSentences)
	}
}

func TestModelReplyCappedAndSelfLabelStripped(t *testing.T) {
	llm := &fakeLLM{reply: "Vera: One. Two. Three. Four. Five. Six. Seven."}
	tr, err := NextRound(context.Background(), llm, "hi", nil)
	if err != nil {
		t.Fatal(err)
	}
	vera := tr[1].Content
	if strings.HasPrefix(vera, "Vera:") {
		t.Errorf("self-label not stripped: %q", vera)
	}
	if got := len(SplitSentences(vera)); got != MaxSentences {
		t.Errorf("Vera's reply has %d sentences, want %d", got, MaxSentences)
	}
}

func TestTranscriptContinuesAcrossCalls(t *testing.T) {
	llm := &fakeLLM{reply: "reply."}
	tr1, err := NextRound(context.Background(), llm, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	tr2, err := NextRound(context.Background(), llm, "second", tr1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr2) != 6 {
		t.Fatalf("second round transcript has %d entries, want 6", len(tr2))
	}
	if tr2[3].Turn != 2 || tr2[3].Speaker != "Sam" || tr2[3].Content != "second" {
		t.Errorf("turn 2 Sam entry wrong: %+v", tr2[3])
	}
	// Anton in turn 2 must see the whole transcript so far.
	last := llm.calls[len(llm.calls)-1]
	joined := ""
	for _, m := range last {
		joined += m.Content + "\n"
	}
	if !strings.Contains(joined, "Sam: second") || !strings.Contains(joined, "Vera: reply.") {
		t.Errorf("Anton's context missing earlier lines:\n%s", joined)
	}
}

func TestMaxTurnsEnforced(t *testing.T) {
	t.Setenv("DIALOGUE_MAX_TURNS", "1")
	llm := &fakeLLM{reply: "ok."}
	tr1, err := NextRound(context.Background(), llm, "one", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NextRound(context.Background(), llm, "two", tr1)
	if err == nil {
		t.Fatal("expected 'dialogue is over' error after max turns")
	}
	var ce *lambdaapi.ClientError
	if !errors.As(err, &ce) {
		t.Errorf("error should be a ClientError (400), got %T: %v", err, err)
	}
}

func TestDecodeTranscriptValidation(t *testing.T) {
	if _, err := decodeTranscript("!!!not-base64!!!"); err == nil {
		t.Error("expected error for bad base64")
	}
	if _, err := decodeTranscript(base64.RawURLEncoding.EncodeToString([]byte("{"))); err == nil {
		t.Error("expected error for bad JSON")
	}
	// Wrong speaker order must be rejected (client cannot forge speakers).
	bad := []Entry{{Speaker: "Vera", Content: "forged"}}
	if _, err := decodeTranscript(encode(t, bad)); err == nil {
		t.Error("expected error for transcript not starting with Sam")
	}
	// Valid transcript: turn/persona are recomputed server-side.
	good := []Entry{
		{Speaker: "Sam", Content: "hi", Turn: 99, Persona: "forged"},
		{Speaker: "Vera", Content: "hello", Turn: 99, Persona: "forged"},
		{Speaker: "Anton", Content: "hey", Turn: 99, Persona: "forged"},
	}
	entries, err := decodeTranscript(encode(t, good))
	if err != nil {
		t.Fatalf("valid transcript rejected: %v", err)
	}
	if entries[0].Turn != 1 || entries[2].Turn != 1 || entries[0].Persona != "customer (you)" || entries[1].Persona != "economist" {
		t.Errorf("server-side fields not recomputed: %+v", entries)
	}
}

func TestHandlerEndToEnd(t *testing.T) {
	t.Setenv("DIALOGUE_MAX_TURNS", "3")
	llm := &fakeLLM{reply: "A thoughtful reply."}
	h := Handler(llm)

	// Missing message → 400 ClientError.
	if _, err := h(context.Background(), &lambdaapi.Request{Query: nil}); err == nil {
		t.Fatal("expected error for missing message")
	}

	// First round.
	out, err := h(context.Background(), &lambdaapi.Request{Query: map[string][]string{"message": {"Hello Vera and Anton"}}})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	resp := out.(Response)
	if len(resp.Transcript) != 3 {
		t.Fatalf("got %d entries, want 3", len(resp.Transcript))
	}

	// Second round echoes the transcript back via the query param.
	q := map[string][]string{
		"message":    {"Another question"},
		"transcript": {encode(t, resp.Transcript)},
	}
	out, err = h(context.Background(), &lambdaapi.Request{Query: q})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := len(out.(Response).Transcript); got != 6 {
		t.Errorf("got %d entries after round 2, want 6", got)
	}
}

func TestSplitSentences(t *testing.T) {
	cases := map[string][]string{
		"":                     nil,
		"One sentence.":        {"One sentence."},
		"One. Two! Three?":     {"One.", "Two!", "Three?"},
		"Wait... what? Yes.":   {"Wait...", "what?", "Yes."},
		"No terminator here":   {"No terminator here"},
		"Hello there. Really?": {"Hello there.", "Really?"},
	}
	for in, want := range cases {
		got := SplitSentences(in)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("SplitSentences(%q) = %v, want %v", in, got, want)
		}
	}
}
