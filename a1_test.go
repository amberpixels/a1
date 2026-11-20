package a1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/amberpixels/a1"
)

// fakeAPI is a scripted /v1/messages endpoint: each call pops the next
// response body. It records every request body for assertions.
type fakeAPI struct {
	t         *testing.T
	responses []string
	requests  []map[string]any
}

func (f *fakeAPI) handler(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Fatalf("decode request: %v", err)
	}
	f.requests = append(f.requests, body)
	if len(f.responses) == 0 {
		f.t.Fatal("fakeAPI: no scripted response left")
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, resp)
}

// newTestClient wires an a1.Client to the scripted endpoint.
func newTestClient(t *testing.T, meter a1.Meter, responses ...string) (*a1.Client, *fakeAPI) {
	t.Helper()
	var opts []a1.Option
	if meter != nil {
		opts = append(opts, a1.WithMeter(meter))
	}
	return newTestClientOpts(t, opts, responses...)
}

// newTestClientOpts is newTestClient for the options a meter cannot express.
func newTestClientOpts(t *testing.T, opts []a1.Option, responses ...string) (*a1.Client, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{t: t, responses: responses}
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	t.Cleanup(srv.Close)
	opts = append([]a1.Option{a1.WithSDKOptions(option.WithBaseURL(srv.URL))}, opts...)
	return a1.NewClient("test-key", opts...), api
}

// message builds a minimal API response with the given text and stop reason.
func message(text, stopReason string) string {
	content := "[]"
	if text != "" {
		b, _ := json.Marshal(text)
		content = fmt.Sprintf(`[{"type":"text","text":%s}]`, b)
	}
	return fmt.Sprintf(`{
		"id":"msg_test","type":"message","role":"assistant",
		"model":"claude-haiku-4-5","content":%s,"stop_reason":%q,
		"usage":{"input_tokens":100,"output_tokens":25}
	}`, content, stopReason)
}

func TestText(t *testing.T) {
	var metered []a1.Meta
	meter := func(_ context.Context, task string, m a1.Meta) {
		if task != "unit" {
			t.Errorf("meter task = %q, want unit", task)
		}
		metered = append(metered, m)
	}
	c, api := newTestClient(t, meter, message("Nice run!", "end_turn"))

	text, meta, err := c.Text(t.Context(), a1.Request{
		Task: "unit", Model: "claude-haiku-4-5", System: "sys", Prompt: "hello",
	})
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	if text != "Nice run!" {
		t.Errorf("text = %q", text)
	}
	if meta.InputTokens != 100 || meta.OutputTokens != 25 || meta.StopReason != "end_turn" {
		t.Errorf("meta = %+v", meta)
	}
	if len(metered) != 1 {
		t.Errorf("meter called %d times, want 1", len(metered))
	}

	req := api.requests[0]
	if req["model"] != "claude-haiku-4-5" {
		t.Errorf("request model = %v", req["model"])
	}
	if req["max_tokens"] != float64(a1.DefaultMaxTokens) {
		t.Errorf("request max_tokens = %v, want default %d", req["max_tokens"], a1.DefaultMaxTokens)
	}
	if _, hasThinking := req["thinking"]; hasThinking {
		t.Error("thinking config sent to a haiku model")
	}
}

func TestTextRefused(t *testing.T) {
	c, _ := newTestClient(t, nil, message("", "refusal"))
	_, _, err := c.Text(t.Context(), a1.Request{Task: "unit", Model: "m", Prompt: "p"})
	if !errors.Is(err, a1.ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
}

func TestTextRetriesEmpty(t *testing.T) {
	c, api := newTestClient(t, nil,
		message("", "end_turn"),       // transient empty answer
		message("second", "end_turn"), // retry succeeds
	)
	text, _, err := c.Text(t.Context(), a1.Request{
		Task: "unit", Model: "m", Prompt: "p", Attempts: 2,
	})
	if err != nil || text != "second" {
		t.Fatalf("text, err = %q, %v — want retry success", text, err)
	}
	if len(api.requests) != 2 {
		t.Errorf("api called %d times, want 2", len(api.requests))
	}
}

func TestTextNoRetryByDefault(t *testing.T) {
	c, api := newTestClient(t, nil, message("", "end_turn"))
	_, _, err := c.Text(t.Context(), a1.Request{Task: "unit", Model: "m", Prompt: "p"})
	if !errors.Is(err, a1.ErrEmpty) {
		t.Fatalf("err = %v, want ErrEmpty", err)
	}
	if len(api.requests) != 1 {
		t.Errorf("api called %d times, want 1", len(api.requests))
	}
}

func TestJSON(t *testing.T) {
	type guess struct {
		Place      string `json:"place"`
		Confidence string `json:"confidence"`
	}
	c, api := newTestClient(t, nil,
		message(`{"place":"strada Ierusalim 5","confidence":"high"}`, "end_turn"))

	schema := a1.Obj(map[string]any{
		"place":      a1.Prop("The place."),
		"confidence": a1.Enum("How sure.", "high", "low"),
	})
	out, _, err := a1.JSON[guess](t.Context(), c, a1.Request{
		Task: "unit", Model: "claude-opus-4-8", Prompt: "p", Thinking: true,
	}, schema)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if out.Place != "strada Ierusalim 5" || out.Confidence != "high" {
		t.Errorf("out = %+v", out)
	}

	req := api.requests[0]
	if _, hasFormat := req["output_config"]; !hasFormat {
		t.Error("output_config missing from request")
	}
	if _, hasThinking := req["thinking"]; !hasThinking {
		t.Error("thinking config missing for an opus model")
	}
}

func TestJSONRetriesGarbled(t *testing.T) {
	type out struct {
		OK string `json:"ok"`
	}
	c, _ := newTestClient(t, nil,
		message(`not json {`, "end_turn"),
		message(`{"ok":"yes"}`, "end_turn"),
	)
	got, _, err := a1.JSON[out](t.Context(), c, a1.Request{
		Task: "unit", Model: "m", Prompt: "p", Attempts: 2,
	}, a1.Obj(map[string]any{"ok": a1.Prop("ok")}))
	if err != nil || got.OK != "yes" {
		t.Fatalf("got, err = %+v, %v — want retry success", got, err)
	}
}

type place struct {
	City string   `json:"city"`
	Tags []string `json:"tags"`
}

var (
	placeSchema  = a1.Obj(map[string]any{"city": a1.Prop("city"), "tags": a1.Prop("tags")})
	errNotACity  = errors.New("not a city")
	cityResponse = func(city string, tags ...string) string {
		b, _ := json.Marshal(place{City: city, Tags: tags})
		return message(string(b), "end_turn")
	}
)

// cityErr is a caller error type, for errors.As through a1's wrapping.
type cityErr struct{ got string }

func (e *cityErr) Error() string { return "bad city " + e.got }

func rejectSentences(p *place) error {
	if strings.Contains(p.City, " ") {
		return errNotACity
	}
	return nil
}

func TestJSONRetriesInvalid(t *testing.T) {
	var metered int
	meter := func(context.Context, string, a1.Meta) { metered++ }
	c, api := newTestClient(t, meter,
		cityResponse("Sure, the city is Chisinau"),
		cityResponse("Chisinau"),
	)
	got, _, err := a1.JSON[place](t.Context(), c, a1.Request{
		Task: "unit", Model: "m", Prompt: "p", Attempts: 2,
	}, placeSchema, a1.Validate(rejectSentences))
	if err != nil || got.City != "Chisinau" {
		t.Fatalf("got, err = %+v, %v - want retry success", got, err)
	}
	if len(api.requests) != 2 || metered != 2 {
		t.Errorf("api calls = %d, meter calls = %d, want 2 and 2", len(api.requests), metered)
	}
}

func TestJSONInvalidExhausted(t *testing.T) {
	c, api := newTestClient(t, nil,
		cityResponse("a b"),
		cityResponse("c d"),
	)
	got, _, err := a1.JSON[place](t.Context(), c, a1.Request{
		Task: "unit", Model: "m", Prompt: "p", Attempts: 2,
	}, placeSchema, a1.Validate(func(p *place) error {
		return fmt.Errorf("%w: %w", errNotACity, &cityErr{got: p.City})
	}))
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
	if !errors.Is(err, a1.ErrInvalid) || !errors.Is(err, errNotACity) {
		t.Errorf("err = %v, want ErrInvalid wrapping errNotACity", err)
	}
	var ce *cityErr
	if !errors.As(err, &ce) || ce.got != "c d" {
		t.Errorf("errors.As cityErr = %v, want the last attempt's", ce)
	}
	if len(api.requests) != 2 {
		t.Errorf("api called %d times, want 2", len(api.requests))
	}
}

func TestJSONValidatePermanent(t *testing.T) {
	c, api := newTestClient(t, nil, cityResponse("a b"))
	_, _, err := a1.JSON[place](t.Context(), c, a1.Request{
		Task: "unit", Model: "m", Prompt: "p", Attempts: 3,
	}, placeSchema, a1.Validate(func(*place) error {
		return a1.Permanent(errNotACity)
	}))
	if !errors.Is(err, a1.ErrInvalid) || !errors.Is(err, errNotACity) {
		t.Errorf("err = %v, want ErrInvalid wrapping errNotACity", err)
	}
	if len(api.requests) != 1 {
		t.Errorf("api called %d times, want 1 (permanent stops the retry)", len(api.requests))
	}
	if a1.Permanent(nil) != nil {
		t.Error("Permanent(nil) != nil")
	}
}

func TestJSONValidateNormalizes(t *testing.T) {
	c, _ := newTestClient(t, nil, cityResponse("  Chisinau "))
	got, _, err := a1.JSON[place](t.Context(), c, a1.Request{
		Task: "unit", Model: "m", Prompt: "p",
	}, placeSchema, a1.Validate(func(p *place) error {
		p.City = strings.TrimSpace(p.City)
		return nil
	}))
	if err != nil || got.City != "Chisinau" {
		t.Fatalf("got, err = %+v, %v - want the trimmed value", got, err)
	}
}

func TestJSONFreshValuePerAttempt(t *testing.T) {
	c, _ := newTestClient(t, nil,
		cityResponse("a b", "stale", "leftover"),
		cityResponse("Chisinau", "fresh"),
	)
	got, _, err := a1.JSON[place](t.Context(), c, a1.Request{
		Task: "unit", Model: "m", Prompt: "p", Attempts: 2,
	}, placeSchema, a1.Validate(rejectSentences))
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if !reflect.DeepEqual(got.Tags, []string{"fresh"}) {
		t.Errorf("tags = %v, want only the second attempt's", got.Tags)
	}
}

func TestJSONLogsRawOnInvalid(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	c, _ := newTestClient(t, func(context.Context, string, a1.Meta) {}, cityResponse("a b"))
	_, _, err := a1.JSON[place](t.Context(), c, a1.Request{
		Task: "unit", Model: "m", Prompt: "p",
	}, placeSchema, a1.Validate(rejectSentences))
	if !errors.Is(err, a1.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	logged := buf.String()
	if !strings.Contains(logged, "a1: unusable response") || !strings.Contains(logged, "msg_test") {
		t.Errorf("log = %q, want the raw response", logged)
	}
	if !strings.Contains(logged, "not a city") {
		t.Errorf("log = %q, want the validation error", logged)
	}
}

func TestObjDefaultsAllRequired(t *testing.T) {
	schema := a1.Obj(map[string]any{"b": a1.Prop("b"), "a": a1.Prop("a")})
	if !reflect.DeepEqual(schema["required"], []string{"a", "b"}) {
		t.Errorf("required = %v, want sorted all-keys", schema["required"])
	}
	if schema["additionalProperties"] != false {
		t.Error("additionalProperties must be false")
	}
	scoped := a1.Obj(map[string]any{"a": a1.Prop("a"), "b": a1.Prop("b")}, "a")
	if !reflect.DeepEqual(scoped["required"], []string{"a"}) {
		t.Errorf("explicit required = %v", scoped["required"])
	}
}

func TestSupportsAdaptiveThinking(t *testing.T) {
	for model, want := range map[string]bool{
		"claude-haiku-4-5":        false,
		"claude-3-haiku-20240307": false,
		"claude-opus-4-8":         true,
		"claude-sonnet-4-6":       true,
	} {
		if got := a1.SupportsAdaptiveThinking(model); got != want {
			t.Errorf("SupportsAdaptiveThinking(%q) = %v, want %v", model, got, want)
		}
	}
}
