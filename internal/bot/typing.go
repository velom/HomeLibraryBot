package bot

import (
	"context"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"
)

// Telegram shows a chat action for ~5s, so it must be re-sent while a slow reply is being prepared.
const typingRefreshInterval = 4 * time.Second

// startTyping calls send immediately and then every interval until the returned stop is called
// or ctx is done. stop blocks until the loop exits and is safe to call more than once.
func startTyping(ctx context.Context, interval time.Duration, send func(context.Context)) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			send(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

// keepTyping shows "печатает…" in the chat until stop is called.
func (b *Bot) keepTyping(ctx context.Context, chatID int64, threadID int) (stop func()) {
	if b.api == nil {
		return func() {}
	}
	return startTyping(ctx, typingRefreshInterval, func(ctx context.Context) {
		_, err := b.api.SendChatAction(ctx, &tgbot.SendChatActionParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Action:          models.ChatActionTyping,
		})
		if err != nil && ctx.Err() == nil {
			b.log(ctx).Debug("Failed to send typing action", zap.Error(err))
		}
	})
}
