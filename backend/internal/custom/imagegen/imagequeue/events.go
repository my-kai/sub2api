package imagequeue

import (
	"context"
	"sync"
)

// TaskEvent 表示生图任务流发生了需要重新读取快照的变化。
//
// Sequence 只用于区分事件先后；SSE 端仍会重新读取数据库快照，避免把内存事件当作最终状态。
type TaskEvent struct {
	Sequence uint64 `json:"sequence"`
}

// TaskEventHub 负责把队列状态变化广播给当前进程内的 SSE 连接。
//
// 队列位置会被其它用户任务的 claim/cancel 间接影响，因此这里使用全局广播；
// 每个 SSE 连接收到事件后只重新读取自己当前 Session 的任务快照。
type TaskEventHub struct {
	mu             sync.Mutex
	nextSequence   uint64
	nextSubscriber int64
	subscribers    map[int64]chan TaskEvent
}

// NewTaskEventHub 创建任务事件中心。
func NewTaskEventHub() *TaskEventHub {
	return &TaskEventHub{subscribers: map[int64]chan TaskEvent{}}
}

// Subscribe 注册一个任务事件订阅；返回的 cleanup 必须在连接结束时调用。
func (h *TaskEventHub) Subscribe(ctx context.Context) (<-chan TaskEvent, func()) {
	if h == nil {
		ch := make(chan TaskEvent)
		close(ch)
		return ch, func() {}
	}

	h.mu.Lock()
	h.nextSubscriber++
	id := h.nextSubscriber
	ch := make(chan TaskEvent, 1)
	h.subscribers[id] = ch
	h.mu.Unlock()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			h.mu.Lock()
			if current, ok := h.subscribers[id]; ok {
				delete(h.subscribers, id)
				close(current)
			}
			h.mu.Unlock()
		})
	}

	go func() {
		<-ctx.Done()
		cleanup()
	}()

	return ch, cleanup
}

// Publish 广播一次任务变化；慢连接只保留最新事件，避免 SSE 客户端积压。
func (h *TaskEventHub) Publish() {
	if h == nil {
		return
	}

	h.mu.Lock()
	h.nextSequence++
	event := TaskEvent{Sequence: h.nextSequence}
	for _, ch := range h.subscribers {
		select {
		case ch <- event:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- event:
			default:
			}
		}
	}
	h.mu.Unlock()
}
