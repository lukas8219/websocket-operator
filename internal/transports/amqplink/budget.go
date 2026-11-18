package amqplink

import "sync"

// All session inboxes share one byte budget; a slow application cannot multiply
// the broker's memory cap by the number of active sessions.
type budget struct {
	mu    sync.Mutex
	used  int
	limit int
}

func (b *budget) take(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n > b.limit-b.used {
		return false
	}
	b.used += n
	return true
}
func (b *budget) release(n int) {
	b.mu.Lock()
	b.used -= n
	b.mu.Unlock()
}
