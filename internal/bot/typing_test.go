package bot

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStartTyping_SendsImmediatelyAndRefreshesUntilStopped(t *testing.T) {
	var sends atomic.Int32
	stop := startTyping(context.Background(), 10*time.Millisecond, func(context.Context) { sends.Add(1) })

	assert.Eventually(t, func() bool { return sends.Load() >= 3 }, time.Second, time.Millisecond)
	stop()
	after := sends.Load()
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, after, sends.Load(), "no sends after stop")
	stop() // idempotent
}

func TestStartTyping_StopsWhenContextDone(t *testing.T) {
	var sends atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	stop := startTyping(ctx, time.Hour, func(context.Context) { sends.Add(1) })
	cancel()
	stop() // must not block once ctx is done
	assert.Equal(t, int32(1), sends.Load())
}

func TestKeepTyping_NoAPIIsNoop(t *testing.T) {
	b, _ := newTestBot(t)
	b.keepTyping(context.Background(), 1, 0)()
}
