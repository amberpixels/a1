// Package a1 is a thin, opinionated layer over the official Anthropic Go SDK
// for the calls amberpixels apps actually make: plain-text completions and
// schema-constrained JSON extraction.
//
// It is deliberately not a framework. The SDK stays visible (models are plain
// id strings, schemas are plain maps) — a1 only owns the glue every app kept
// rewriting:
//
//   - usage metering: every billed call reports Meta (model, tokens, duration)
//     through one gate — by default the grep-able "💥 tokens burned" log line
//   - stop-reason handling: refusal and max_tokens truncation become typed
//     errors instead of silently wrong text
//   - response assembly: text blocks are concatenated (answers have been
//     observed split across blocks or followed by empty ones)
//   - retry: transient empty/truncated/garbled responses — a failure class the
//     SDK's transport retries don't cover — are retried when Request.Attempts
//     allows; API errors are never retried here (the SDK already does that)
package a1

import (
	"context"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// DefaultMaxTokens bounds a completion when Request.MaxTokens is unset.
const DefaultMaxTokens = 4096

// Meta is the usage/latency record of one billed API call.
type Meta struct {
	Model        string
	StopReason   string
	InputTokens  int64
	OutputTokens int64
	Duration     time.Duration
}

// Meter observes every billed call (including failed attempts that still
// returned a response). task is Request.Task.
type Meter func(ctx context.Context, task string, m Meta)

// Client wraps the Anthropic SDK client with metering.
type Client struct {
	api   anthropic.Client
	meter Meter
}

// Option configures a Client.
type Option func(*config)

type config struct {
	meter   Meter
	sdkOpts []option.RequestOption
}

// WithMeter replaces the default metering log line with a custom observer
// (e.g. the app's own logger, or a metrics counter).
func WithMeter(m Meter) Option {
	return func(c *config) { c.meter = m }
}

// WithSDKOptions appends raw SDK request options to the underlying client —
// base URL overrides, custom HTTP clients, extra headers.
func WithSDKOptions(opts ...option.RequestOption) Option {
	return func(c *config) { c.sdkOpts = append(c.sdkOpts, opts...) }
}

// NewClient builds a metered client for the given API key.
func NewClient(apiKey string, opts ...Option) *Client {
	cfg := config{meter: defaultMeter}
	for _, opt := range opts {
		opt(&cfg)
	}
	sdkOpts := append([]option.RequestOption{option.WithAPIKey(apiKey)}, cfg.sdkOpts...)
	return &Client{
		api:   anthropic.NewClient(sdkOpts...),
		meter: cfg.meter,
	}
}

// defaultMeter is the shared cost-audit convention: paid LLM activity —
// foreground or background worker — is always one `grep 💥` away.
func defaultMeter(ctx context.Context, task string, m Meta) {
	slog.InfoContext(ctx, "💥 tokens burned (Anthropic)",
		"task", task,
		"model", m.Model,
		"input_tokens", m.InputTokens,
		"output_tokens", m.OutputTokens,
		"duration", m.Duration.Round(time.Millisecond),
	)
}

// Request describes one completion call. Model is a plain Anthropic model id
// (e.g. "claude-haiku-4-5" or an anthropic.ModelClaude* constant).
type Request struct {
	// Task names the call site for metering/logs (e.g. "geo-guess",
	// "compose-message").
	Task string
	// Model is the Anthropic model id. Required.
	Model string
	// System is the system prompt; empty means none.
	System string
	// Prompt is the user message.
	Prompt string
	// MaxTokens bounds the completion; 0 means DefaultMaxTokens.
	MaxTokens int64
	// Thinking enables adaptive thinking on models that support it (it is
	// silently skipped on models that don't, e.g. Haiku 4.5).
	Thinking bool
	// Attempts is how many times a transient content failure (empty,
	// truncated, or unparseable response) is tried before giving up.
	// 0 or 1 means no retry. API/transport errors are never retried here.
	Attempts int
}
