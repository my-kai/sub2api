package promptauditv2

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
)

var (
	// errWorkerPoolUnavailable is internal because gateway callers receive a stable public error code.
	errWorkerPoolUnavailable = errors.New("prompt audit v2 worker pool is unavailable")
	// errWorkerQueueFull is internal because gateway callers receive a stable public error code.
	errWorkerQueueFull = errors.New("prompt audit v2 worker queue is full")
)

type auditTask struct {
	ctx     context.Context
	request securityaudit.Request
	message string
	result  chan securityaudit.Decision
}

// WorkerPool bounds both concurrent model calls and queued gateway requests for
// one immutable runtime configuration generation.
type WorkerPool struct {
	queue      chan auditTask
	stop       chan struct{}
	process    func(context.Context, securityaudit.Request, string) securityaudit.Decision
	mu         sync.RWMutex
	accepting  bool
	workers    sync.WaitGroup
	processing atomic.Int64
}

// NewWorkerPool starts a fixed number of workers and refuses invalid capacities.
func NewWorkerPool(workerCount, queueCapacity int, process func(context.Context, securityaudit.Request, string) securityaudit.Decision) (*WorkerPool, error) {
	if workerCount <= 0 || queueCapacity <= 0 || process == nil {
		return nil, errWorkerPoolUnavailable
	}
	pool := &WorkerPool{
		queue: make(chan auditTask, queueCapacity), stop: make(chan struct{}),
		process: process, accepting: true,
	}
	pool.workers.Add(workerCount)
	for index := 0; index < workerCount; index++ {
		go pool.runWorker()
	}
	return pool, nil
}

// Submit enqueues without blocking when capacity is exhausted, then waits for
// the authoritative result or request cancellation.
func (p *WorkerPool) Submit(ctx context.Context, request securityaudit.Request, message string) (securityaudit.Decision, error) {
	if p == nil {
		return securityaudit.Decision{}, errWorkerPoolUnavailable
	}
	task := auditTask{ctx: ctx, request: request.Clone(), message: message, result: make(chan securityaudit.Decision, 1)}
	p.mu.RLock()
	if !p.accepting {
		p.mu.RUnlock()
		return securityaudit.Decision{}, errWorkerPoolUnavailable
	}
	select {
	case p.queue <- task:
		p.mu.RUnlock()
	case <-ctx.Done():
		p.mu.RUnlock()
		return securityaudit.Decision{}, ctx.Err()
	default:
		p.mu.RUnlock()
		return securityaudit.Decision{}, errWorkerQueueFull
	}
	select {
	case decision := <-task.result:
		return decision, nil
	case <-ctx.Done():
		return securityaudit.Decision{}, ctx.Err()
	}
}

// StopAccepting prevents new submissions and drains every task already accepted.
func (p *WorkerPool) StopAccepting() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.accepting {
		p.accepting = false
		close(p.stop)
	}
	p.mu.Unlock()
}

// Wait blocks until every worker has drained the accepted queue.
func (p *WorkerPool) Wait() { p.workers.Wait() }

// Stats returns a stable snapshot without exposing queued request content.
func (p *WorkerPool) Stats() (queued int, processing int64) {
	if p == nil {
		return 0, 0
	}
	return len(p.queue), p.processing.Load()
}

func (p *WorkerPool) runWorker() {
	defer p.workers.Done()
	for {
		select {
		case task := <-p.queue:
			p.execute(task)
		case <-p.stop:
			for {
				select {
				case task := <-p.queue:
					p.execute(task)
				default:
					return
				}
			}
		}
	}
}

func (p *WorkerPool) execute(task auditTask) {
	if task.ctx.Err() != nil {
		return
	}
	p.processing.Add(1)
	decision := p.process(task.ctx, task.request, task.message)
	p.processing.Add(-1)
	select {
	case task.result <- decision:
	default:
		// The gateway request was canceled after processing. The buffered channel
		// prevents workers from blocking while preserving completed side effects.
	}
}
