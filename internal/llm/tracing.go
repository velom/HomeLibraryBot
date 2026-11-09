package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"library/internal/telemetry"
)

// Spans in this file use the Langfuse attribute mapping:
// https://langfuse.com/integrations/native/opentelemetry#property-mapping
const (
	attrObsType       = "langfuse.observation.type"
	attrObsInput      = "langfuse.observation.input"
	attrObsOutput     = "langfuse.observation.output"
	attrObsLevel      = "langfuse.observation.level"
	attrObsStatusMsg  = "langfuse.observation.status_message"
	attrObsModel      = "langfuse.observation.model.name"
	attrObsUsage      = "langfuse.observation.usage_details"
	attrObsMetaPrefix = "langfuse.observation.metadata."
	attrSessionID     = "langfuse.session.id"
	attrUserID        = "langfuse.user.id"
	attrTraceName     = "langfuse.trace.name"
	attrTraceTags     = "langfuse.trace.tags"
	attrEnvironment   = "langfuse.environment"

	generationName = "generate-response"
)

// tracer returns the Langfuse-scoped tracer from the current global provider.
func tracer() trace.Tracer { return otel.Tracer(telemetry.LangfuseScope) }

// TraceAttrs are trace-level Langfuse attributes. They are put on every observation
// of the trace (Langfuse filters per observation) and, in gateway mode, sent as
// langfuse-* request headers so the gateway's generations carry them too.
type TraceAttrs struct {
	SessionID   string
	UserID      string
	TraceName   string
	Environment string
	Tags        []string
}

type traceAttrsKey struct{}

func traceAttrsFrom(ctx context.Context) TraceAttrs {
	a, _ := ctx.Value(traceAttrsKey{}).(TraceAttrs)
	return a
}

func (a TraceAttrs) keyValues() []attribute.KeyValue {
	var kvs []attribute.KeyValue
	add := func(k, v string) {
		if v != "" {
			kvs = append(kvs, attribute.String(k, v))
		}
	}
	add(attrSessionID, a.SessionID)
	add(attrUserID, a.UserID)
	add(attrTraceName, a.TraceName)
	add(attrEnvironment, a.Environment)
	if len(a.Tags) > 0 {
		kvs = append(kvs, attribute.StringSlice(attrTraceTags, a.Tags))
	}
	return kvs
}

// gatewayHeaders sets the Langfuse AI Gateway context headers:
// https://langfuse.com/docs/gateway/features/tracing#request-headers
func (a TraceAttrs) setGatewayHeaders(h http.Header) {
	set := func(k, v string) {
		if v != "" {
			h.Set(k, gatewayEscape(v))
		}
	}
	set("langfuse-session-id", a.SessionID)
	set("langfuse-user-id", a.UserID)
	set("langfuse-trace-name", a.TraceName)
	set("langfuse-environment", a.Environment)
	if len(a.Tags) > 0 {
		tags := make([]string, len(a.Tags))
		for i, t := range a.Tags {
			tags[i] = gatewayEscape(t)
		}
		h.Set("langfuse-tags", strings.Join(tags, ","))
	}
}

// gatewayEscape percent-encodes the characters the gateway decodes or splits on.
func gatewayEscape(v string) string {
	return strings.NewReplacer("%", "%25", ",", "%2C").Replace(v)
}

// injectTraceparent makes the gateway record its generation inside the caller's trace,
// nested under the span active in ctx.
func injectTraceparent(ctx context.Context, h http.Header) {
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(h))
}

// Observation is a Langfuse observation backed by an OTel span. A nil *Observation is a no-op.
type Observation struct {
	span trace.Span
}

func startObservation(ctx context.Context, name, obsType string, extra ...attribute.KeyValue) (context.Context, *Observation) {
	attrs := append([]attribute.KeyValue{attribute.String(attrObsType, obsType)}, traceAttrsFrom(ctx).keyValues()...)
	attrs = append(attrs, extra...)
	ctx, span := tracer().Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, &Observation{span: span}
}

// StartAgent starts the root observation of one agent run (one trace in Langfuse).
// attrs are attached to this and every nested observation started from the returned context.
func StartAgent(ctx context.Context, name string, attrs TraceAttrs, input string) (context.Context, *Observation) {
	ctx = context.WithValue(ctx, traceAttrsKey{}, attrs)
	return startObservation(ctx, name, "agent",
		telemetry.LangfuseRootAttr.Bool(true),
		attribute.String(attrObsInput, input),
	)
}

// StartTool starts a tool-call observation; input is the tool arguments JSON.
func StartTool(ctx context.Context, name, input string) (context.Context, *Observation) {
	return startObservation(ctx, name, "tool", attribute.String(attrObsInput, input))
}

func startGeneration(ctx context.Context, model string, messages []Message, tools []Tool) (context.Context, *Observation) {
	attrs := []attribute.KeyValue{attribute.String(attrObsModel, model)}
	if in, err := json.Marshal(messages); err == nil {
		attrs = append(attrs, attribute.String(attrObsInput, string(in)))
	}
	if len(tools) > 0 {
		names := make([]string, len(tools))
		for i, t := range tools {
			names[i] = t.Function.Name
		}
		attrs = append(attrs, attribute.String(attrObsMetaPrefix+"tools", strings.Join(names, ",")))
	}
	return startObservation(ctx, generationName, "generation", attrs...)
}

// SetOutput records the observation output (plain text or a JSON string).
func (o *Observation) SetOutput(output string) {
	if o == nil {
		return
	}
	o.span.SetAttributes(attribute.String(attrObsOutput, output))
}

// SetMetadata records a top-level, filterable metadata key.
func (o *Observation) SetMetadata(key string, value any) {
	if o == nil {
		return
	}
	o.span.SetAttributes(attribute.String(attrObsMetaPrefix+key, fmt.Sprint(value)))
}

// Fail marks the observation as failed (level ERROR in Langfuse).
func (o *Observation) Fail(err error) {
	if o == nil || err == nil {
		return
	}
	o.span.RecordError(err)
	o.span.SetStatus(codes.Error, err.Error())
	o.span.SetAttributes(
		attribute.String(attrObsLevel, "ERROR"),
		attribute.String(attrObsStatusMsg, err.Error()),
	)
}

// End finishes the observation.
func (o *Observation) End() {
	if o == nil {
		return
	}
	o.span.End()
}

func (o *Observation) setUsage(u *usage) {
	if o == nil || u == nil {
		return
	}
	if b, err := json.Marshal(u.details()); err == nil {
		o.span.SetAttributes(attribute.String(attrObsUsage, string(b)))
	}
}

// usage is the OpenAI chat completions usage object.
type usage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details,omitempty"`
}

// details converts OpenAI's inclusive counts into Langfuse's mutually exclusive usage buckets:
// https://langfuse.com/docs/observability/features/token-and-cost-tracking#usage-details-contract
func (u *usage) details() map[string]int {
	var cached, reasoning int
	if u.PromptTokensDetails != nil {
		cached = min(u.PromptTokensDetails.CachedTokens, u.PromptTokens)
	}
	if u.CompletionTokensDetails != nil {
		reasoning = min(u.CompletionTokensDetails.ReasoningTokens, u.CompletionTokens)
	}
	d := map[string]int{
		"input":  u.PromptTokens - cached,
		"output": u.CompletionTokens - reasoning,
		"total":  u.TotalTokens,
	}
	if d["total"] == 0 {
		d["total"] = u.PromptTokens + u.CompletionTokens
	}
	if cached > 0 {
		d["input_cached_tokens"] = cached
	}
	if reasoning > 0 {
		d["output_reasoning_tokens"] = reasoning
	}
	return d
}
