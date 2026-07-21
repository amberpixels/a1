<p align="center">
  <img src="logo.svg" alt="a1" width="230">
</p>

<div align="center">

### Grade-A1 AI calls for Go.

A thin, opinionated layer over the official Anthropic Go SDK - plain-text completions and schema-constrained JSON extraction.

[![Go Reference](https://pkg.go.dev/badge/github.com/amberpixels/a1.svg)](https://pkg.go.dev/github.com/amberpixels/a1)
[![Go Version](https://img.shields.io/github/go-mod/go-version/amberpixels/a1)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg)](LICENSE)

</div>

---

`a1` wraps the [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go) for the two calls apps actually make: **plain-text completions** and **schema-constrained JSON extraction**.

It is deliberately **not a framework**. Models stay plain id strings, schemas stay plain maps, the SDK stays one import away. `a1` only owns the glue you keep rewriting between projects:

- **Metering** - every billed call reports model, tokens, and duration through one gate; the default is a grep-able log line (`grep 💥` = everything that cost money)
- **Stop-reason handling** - refusals and `max_tokens` truncation become typed errors (`ErrRefused`, `ErrTruncated`) instead of silently wrong text
- **Response assembly** - text blocks are concatenated (answers get split across blocks in the wild)
- **Content retry** - transient empty/truncated/garbled responses are retried; API errors are not (the SDK already retries those)

## Install

```bash
go get github.com/amberpixels/a1
```

## Plain Text

```go
c := a1.NewClient(os.Getenv("ANTHROPIC_API_KEY"))

text, meta, err := c.Text(ctx, a1.Request{
    Task:      "compose-message", // names the call site in metering logs
    Model:     "claude-haiku-4-5",
    System:    "You write Telegram notifications. Plain text only.",
    Prompt:    prompt,
    MaxTokens: 1024,
})
// meta: Model, StopReason, InputTokens, OutputTokens, Duration
```

## Structured JSON

`JSON[T]` uses Anthropic's native structured outputs - the response is schema-guaranteed, then unmarshaled into your type. Keep property descriptions rich: the model reads them like prompt text.

```go
type Guess struct {
    Place      string `json:"place"`
    Confidence string `json:"confidence"`
}

schema := a1.Obj(map[string]any{
    "place":      a1.Prop("A single geocodable place in the local language."),
    "confidence": a1.Enum("How sure the answer is.", "high", "medium", "low"),
})

guess, meta, err := a1.JSON[Guess](ctx, c, a1.Request{
    Task:     "geo-guess",
    Model:    "claude-opus-4-8",
    System:   systemPrompt,
    Prompt:   prompt,
    Thinking: true, // adaptive thinking; silently skipped on models without it
    Attempts: 2,    // retry transient empty/garbled responses
}, schema)
```

`a1.Obj` requires every property by default (structured outputs reject optional-by-absence anyway) and always sets `additionalProperties: false`.

## Metering

The default meter logs `💥 tokens burned (Anthropic)` via `slog`. Plug in your own logger or metrics:

```go
c := a1.NewClient(key, a1.WithMeter(func(ctx context.Context, task string, m a1.Meta) {
    myLogger.Info("💥 tokens burned", "task", task, "model", m.Model,
        "in", m.InputTokens, "out", m.OutputTokens)
}))
```

The meter fires on **every billed call**, including failed attempts that still returned a response - it tracks spend, not success.

## Errors

```go
text, _, err := c.Text(ctx, req)
switch {
case errors.Is(err, a1.ErrRefused):    // model/classifier declined - don't retry
case errors.Is(err, a1.ErrTruncated):  // raise MaxTokens
case errors.Is(err, a1.ErrEmpty):      // transient; raise Attempts
case err != nil:                       // SDK error (auth, rate limit, network...)
}
```

## Testing Your Integration

`WithSDKOptions` accepts raw SDK options - point the client at an `httptest` server and script responses (see [`a1_test.go`](a1_test.go) for a ready-made pattern):

```go
c := a1.NewClient("test-key", a1.WithSDKOptions(option.WithBaseURL(srv.URL)))
```

## Feedback

a1 is a solo, opinionated project - but if you stumbled upon it and have
ideas, questions, or bug reports, an [issue](https://github.com/amberpixels/a1/issues) is always welcome :)

## License

[MIT](LICENSE) © [amberpixels](https://amberpixels.io)
