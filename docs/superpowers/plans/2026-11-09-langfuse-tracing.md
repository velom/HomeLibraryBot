# Langfuse Tracing for /ask Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Send each /ask turn to Langfuse as a well-structured trace (agent → generation/tool observations, sessions, users, tokens), either via direct OTLP export or via the Langfuse AI Gateway.

**Architecture:** Go has no Langfuse SDK, so we use the OTel API with `langfuse.*` attributes (docs: integrations/native/opentelemetry). A second span processor on the existing global TracerProvider exports **only** spans from instrumentation scope `library/llm` to `<LANGFUSE_BASE_URL>/api/public/otel/v1/traces` (Basic auth + `x-langfuse-ingestion-version: 4`). The existing ClickStack OTLP export is unchanged and still sees everything (same trace IDs → cross-system correlation). The /ask root `agent` span is a child of `telegram.message /ask`, which is filtered out of Langfuse; the exporter clears the parent of spans marked as Langfuse roots so they don't become orphans. Gateway mode (`LLM_LANGFUSE_GATEWAY=true`): the gateway already records each model call as a generation, so the app skips its own generation span (docs: don't double-record) and instead sends `traceparent` (of the agent span) + `langfuse-*` headers so the gateway nests its generation inside our trace.

**Tech Stack:** Go 1.26, go.opentelemetry.io/otel v1.47 (sdk/trace, otlptracehttp), otelhttp v0.72.

**Spec:** user request + Langfuse docs fetched 2026-11-09 (best-practices, opentelemetry attribute mapping, token-and-cost-tracking, gateway tracing).

## Global Constraints

- Env: `LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY`, `LANGFUSE_BASE_URL` (default `https://cloud.langfuse.com`), `LANGFUSE_TRACING_ENVIRONMENT` (optional), `LLM_LANGFUSE_GATEWAY` (bool, default false). Langfuse export enabled iff both keys set.
- Langfuse export is independent of `OTEL_EXPORTER_OTLP_ENDPOINT`: either, both, or neither may be configured. Metrics/logs only exist with OTLP.
- Names are stable, verb-first, no dynamic values: trace/agent `answer-question`, generation `generate-response`, tool spans named by tool name (`get_books`, …).
- Never send secrets (LLM key, bot token) into attributes. LLM HTTP calls must not propagate `traceparent`/`baggage` to the provider except the explicit gateway `traceparent`.
- Usage stored as exclusive flat buckets: `input = prompt − cached`, `input_cached_tokens`, `output = completion − reasoning`, `output_reasoning_tokens`, `total`. Omit zero detail keys.

## Review Focus

- Langfuse configured but OTLP not: TracerProvider must still be installed (today Setup returns noop) → test in Task 1.
- Turn hits `maxToolIterations`: root output must be the limit message and metadata `hit_iteration_limit=true` → test in Task 3.
- LLM HTTP error / non-200: generation span level `ERROR` with status message; root span also ERROR → test in Task 2/3.
- Gateway mode: no `generate-response` span emitted, request carries `traceparent` equal to agent span context and `langfuse-session-id` header → test in Task 2.
- Response without `usage` (some compat providers): no usage attribute, no panic → test in Task 2.

---

### Task 1: Langfuse exporter in telemetry

**Files:**
- Create: `internal/telemetry/langfuse.go`, `internal/telemetry/langfuse_test.go`
- Modify: `internal/telemetry/telemetry.go` (Setup), `internal/app/app.go:60` (log field `langfuse_export`)

**Interfaces:**
- Produces: `const LangfuseScope = "library/llm"`; `const LangfuseRootAttr = "library.langfuse.root"` (bool marker); `func LangfuseEnabled() bool`; `func newLangfuseExporter(ctx) (sdktrace.SpanExporter, error)`; `type langfuseFilter struct{ next sdktrace.SpanExporter }` implementing SpanExporter.

- [ ] Test `TestLangfuseFilterKeepsOnlyLLMScope`: record spans from scopes `library/llm` and `library/internal/bot` through `langfuseFilter{next: tracetest.NewInMemoryExporter()}`; assert only the `library/llm` span is exported.
- [ ] Test `TestLangfuseFilterDetachesRoot`: span in `library/llm` with `LangfuseRootAttr=true` and a parent → exported span has invalid `Parent()` and no `LangfuseRootAttr` attribute; a non-root child keeps its parent.
- [ ] Test `TestSetupLangfuseOnly`: OTLP endpoint empty, keys set, base URL = httptest server → after Setup, `otel.GetTracerProvider()` is an `*sdktrace.TracerProvider`; start/end a `library/llm` span, shutdown; server received POST `/api/public/otel/v1/traces` with `Authorization: Basic base64(pk:sk)` and `x-langfuse-ingestion-version: 4`.
- [ ] Implement: exporter via `otlptracehttp.New(ctx, WithEndpointURL(base+"/api/public/otel/v1/traces"), WithHeaders(...))`. Root detach via a wrapper struct embedding `sdktrace.ReadOnlySpan` overriding `Parent()` and `Attributes()`. Setup: build TracerProvider when `Enabled() || LangfuseEnabled()`; add OTLP batcher only if `Enabled()`, Langfuse batcher only if `LangfuseEnabled()`; propagator always set when tp is built. Metrics/logs/runtime only if `Enabled()`.
- [ ] `go test ./internal/telemetry/...` passes; commit.

### Task 2: Generation tracing + gateway mode in llm client

**Files:**
- Create: `internal/llm/tracing.go`, `internal/llm/tracing_test.go`
- Modify: `internal/llm/client.go` (Config.Gateway, ChatWithTools, response usage/model parsing, otelhttp without propagation)

**Interfaces:**
- Produces:
  - `type TraceAttrs struct{ SessionID, UserID, TraceName, Environment string; Tags []string }`
  - `func StartAgent(ctx, name string, attrs TraceAttrs, input string) (context.Context, *Observation)` — root, type `agent`, marker attr, trace attrs set + stored in ctx.
  - `func StartTool(ctx, name, input string) (context.Context, *Observation)` — type `tool`.
  - `(*Observation).SetOutput(string)`, `.SetMetadata(key string, v any)`, `.Fail(err error)`, `.End()`.
  - Every span from these helpers (and the generation) carries the ctx TraceAttrs (`langfuse.session.id`, `langfuse.user.id`, `langfuse.trace.name`, `langfuse.trace.tags`, `langfuse.environment`) per "propagate trace attributes to all spans".
  - `Config.Gateway bool`.
- [ ] Test `TestChatWithToolsRecordsGeneration` (in-memory tracer provider, httptest LLM returning tool_calls + `usage{prompt_tokens:100, completion_tokens:20, total_tokens:120, prompt_tokens_details:{cached_tokens:30}, completion_tokens_details:{reasoning_tokens:5}}`): one span `generate-response`, `langfuse.observation.type=generation`, `langfuse.observation.model.name=test-model`, usage_details JSON `{"input":70,"input_cached_tokens":30,"output":15,"output_reasoning_tokens":5,"total":120}`, input = JSON messages array, output = raw assistant message JSON containing `tool_calls`, session id attr from ctx.
- [ ] Test `TestChatWithToolsNoUsage`: response without usage → no `langfuse.observation.usage_details` attr.
- [ ] Test `TestChatWithToolsErrorMarksGeneration`: 500 response → span status Error, `langfuse.observation.level=ERROR`.
- [ ] Test `TestGatewayModeSkipsGenerationAndSendsContext`: Gateway=true, inside StartAgent ctx → server sees `traceparent` with agent trace/span id, `langfuse-session-id`, `langfuse-user-id`, `langfuse-tags`, `langfuse-trace-name`; no `generate-response` span recorded.
- [ ] Test `TestNoTraceparentToProvider`: Gateway=false with global propagator set → request has no `traceparent` header.
- [ ] Implement; tracer = `otel.Tracer(telemetry.LangfuseScope)` (import telemetry const; no cycle). Tool definitions go to generation metadata `tools` (names list) to keep input a renderable message list. Model params: none sent today, skip.
- [ ] `go test ./internal/llm/...`; commit.

### Task 3: Instrument /ask agent loop + sessions

**Files:**
- Modify: `internal/bot/ask.go`, `internal/config/config.go`, `internal/app/app.go`, `.env.example`, `.github/workflows/deploy.yml`, `CLAUDE.md`
- Test: `internal/bot/ask_test.go` (currently empty `package bot`)

**Interfaces:**
- Consumes: Task 2 helpers.
- [ ] Session ID: generated in `handleAsk` (`ask-` + uuid v4 via github.com/google/uuid), stored in `state.Data["session_id"]`; follow-ups reuse it. UserID = Telegram user ID as decimal string. Tags `["ask"]`. TraceName `answer-question`. Environment from config.
- [ ] `runAskWithTools(ctx, history, attrs llm.TraceAttrs)`: StartAgent(input = latest user message text); each tool call wrapped in StartTool(name, args JSON) with output = result string, and `Fail` when result starts with `error:`; agent output = final answer; metadata `tool_iterations`, `hit_iteration_limit`; on LLM error `Fail(err)`.
- [ ] Test `TestRunAskWithToolsTrace` using in-memory tracer provider, stub storage (`stubs`), fake LLM httptest server (1st reply tool_call get_books, 2nd reply text): spans `answer-question`(agent, input=question, output=answer), `get_books`(tool, parent = agent), 2× `generate-response`(parent = agent); all carry session id.
- [ ] Test `TestRunAskWithToolsIterationLimit`: LLM always returns tool_calls → agent metadata `hit_iteration_limit=true`.
- [ ] Config: `LangfuseEnvironment` (LANGFUSE_TRACING_ENVIRONMENT), `LLMLangfuseGateway` (LLM_LANGFUSE_GATEWAY). Pass Gateway to llm.Config; env to bot via NewBot/field. Docs: `.env.example`, deploy.yml (LANGFUSE_* secrets), CLAUDE.md config section.
- [ ] `make build && make test`; commit.
