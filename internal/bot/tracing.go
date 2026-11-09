package bot

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"library/internal/telemetry"
)

var tracer = otel.Tracer("library/internal/bot")

// log returns the bot logger enriched with trace_id/span_id from ctx.
func (b *Bot) log(ctx context.Context) *zap.Logger {
	return telemetry.Logger(ctx, b.logger)
}

// startUpdateSpan starts the root span for an incoming Telegram update.
func startUpdateSpan(ctx context.Context, update *models.Update) (context.Context, trace.Span) {
	name := "telegram.update"
	attrs := []attribute.KeyValue{attribute.Int64("telegram.update_id", update.ID)}
	switch {
	case update.Message != nil:
		name = "telegram.message"
		if cmd := commandOf(update.Message.Text); cmd != "" {
			name += " " + cmd
			attrs = append(attrs, attribute.String("telegram.command", cmd))
		}
	case update.CallbackQuery != nil:
		name = "telegram.callback"
		if action, _, _ := strings.Cut(update.CallbackQuery.Data, ":"); action != "" {
			attrs = append(attrs, attribute.String("telegram.callback.action", action))
		}
	}
	return tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attrs...))
}

func commandOf(text string) string {
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	cmd, _, _ := strings.Cut(strings.Fields(text)[0], "@")
	return cmd
}

// telegramClient records a client span per Telegram Bot API call. It is used
// instead of otelhttp because the request URL embeds the bot token, which must
// never reach telemetry; for the same reason it redacts the URL in returned
// errors, which are logged (and exported) by callers. Long-poll getUpdates calls
// are not traced.
type telegramClient struct {
	client *http.Client
}

func newTelegramClient() telegramClient {
	return telegramClient{client: &http.Client{Timeout: telegramPollTimeout}}
}

// telegramPollTimeout mirrors the go-telegram/bot default.
const telegramPollTimeout = time.Minute

func (c telegramClient) Do(req *http.Request) (*http.Response, error) {
	method := path.Base(req.URL.Path)
	if method == "getUpdates" {
		resp, err := c.client.Do(req)
		return resp, redactURL(err, method)
	}
	ctx, span := tracer.Start(req.Context(), "telegram "+method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("rpc.system", "telegram"),
			attribute.String("rpc.method", method),
			attribute.String("server.address", req.URL.Hostname()),
		))
	defer span.End()

	resp, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		err = redactURL(err, method)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, resp.Status)
	}
	return resp, nil
}

func redactURL(err error, method string) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return &url.Error{Op: ue.Op, URL: "telegram:" + method, Err: ue.Err}
	}
	return err
}
