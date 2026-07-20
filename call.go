package a1

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// Text runs one plain-text completion and returns the assistant's text.
func (c *Client) Text(ctx context.Context, req Request) (string, Meta, error) {
	var (
		text string
		meta Meta
	)
	err := c.withRetry(ctx, req, func() error {
		var err error
		text, meta, err = c.once(ctx, req, nil)
		return err
	})
	return text, meta, err
}

// JSON runs one schema-constrained completion (structured outputs) and
// unmarshals the response into T. The schema is a plain JSON-schema map —
// build it with Obj/Prop/Enum or by hand; keep property descriptions rich,
// they steer the model as much as the prompt does.
func JSON[T any](ctx context.Context, c *Client, req Request, schema map[string]any) (*T, Meta, error) {
	var (
		out  T
		meta Meta
	)
	err := c.withRetry(ctx, req, func() error {
		text, m, err := c.once(ctx, req, schema)
		meta = m
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			// Garbled JSON is transient (same class as an empty answer) —
			// mark it retryable.
			return retryable{fmt.Errorf("a1: decoding response: %w", err)}
		}
		return nil
	})
	if err != nil {
		return nil, meta, err
	}
	return &out, meta, nil
}

// withRetry runs fn up to req.Attempts times, stopping early on
// non-retryable errors or context cancellation.
func (c *Client) withRetry(ctx context.Context, req Request, fn func() error) error {
	attempts := max(req.Attempts, 1)
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		lastErr = fn()
		if lastErr == nil || !isRetryable(lastErr) || ctx.Err() != nil {
			return lastErr
		}
	}
	return lastErr
}

// once performs a single billed API call: build params, call, meter, map the
// stop reason, assemble the text.
func (c *Client) once(ctx context.Context, req Request, schema map[string]any) (string, Meta, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = DefaultMaxTokens
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt)),
		},
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}
	if schema != nil {
		params.OutputConfig = anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		}
	}
	if req.Thinking && SupportsAdaptiveThinking(req.Model) {
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		}
	}

	start := time.Now()
	msg, err := c.api.Messages.New(ctx, params)
	if err != nil {
		return "", Meta{Model: req.Model}, err
	}
	meta := Meta{
		Model:        msg.Model,
		StopReason:   string(msg.StopReason),
		InputTokens:  msg.Usage.InputTokens,
		OutputTokens: msg.Usage.OutputTokens,
		Duration:     time.Since(start),
	}
	c.meter(ctx, req.Task, meta)

	switch msg.StopReason {
	case anthropic.StopReasonRefusal:
		return "", meta, ErrRefused
	case anthropic.StopReasonMaxTokens:
		return "", meta, retryable{ErrTruncated}
	}

	// Concatenate every text block: the answer is normally one block, but it
	// has been observed split across blocks or followed by empty ones.
	var b strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		logRawResponse(ctx, req.Task, msg)
		return "", meta, retryable{ErrEmpty}
	}
	return text, meta, nil
}

// logRawResponse dumps the (truncated) raw API response when it carried no
// usable answer, so unexpected shapes can be diagnosed from the logs.
func logRawResponse(ctx context.Context, task string, msg *anthropic.Message) {
	raw := msg.RawJSON()
	if len(raw) > 4000 {
		raw = raw[:4000] + "...(truncated)"
	}
	slog.WarnContext(ctx, "a1: unusable response", "task", task, "raw", raw)
}

// SupportsAdaptiveThinking reports whether a model accepts the adaptive
// thinking config. Haiku-tier and pre-4.x models reject it.
func SupportsAdaptiveThinking(model string) bool {
	return !strings.Contains(model, "haiku") && !strings.HasPrefix(model, "claude-3")
}
