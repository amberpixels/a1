package a1_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	fmt.Fprint(w, resp)
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
