package app

import (
	"context"
	"sync"
	"time"
)

const playbackProgressTimeout = 8 * time.Second

type playbackProgressReport struct {
	server             *Server
	item               MediaItem
	position, duration float64
	timeout            time.Duration
	session            uint64
	done               chan error
	barrier            bool
	onError            func(error)
}

// playbackProgressReporter sends progress updates in one order. There is at
// most one queued periodic snapshot per session; final reports are retained.
type playbackProgressReporter struct {
	mu            sync.Mutex
	wake          *sync.Cond
	queue         []playbackProgressReport
	closed        bool
	currentCancel context.CancelFunc
	currentDone   chan error
	workerDone    chan struct{}
	nextSession   uint64
}

type playbackProgressSession struct {
	mu      sync.Mutex
	closed  bool
	id      uint64
	report  *playbackProgressReporter
	server  *Server
	item    MediaItem
	onError func(error)
}

func newPlaybackProgressReporter() *playbackProgressReporter {
	r := &playbackProgressReporter{workerDone: make(chan struct{})}
	r.wake = sync.NewCond(&r.mu)
	go r.run()
	return r
}

func (r *playbackProgressReporter) newSession(server *Server, item MediaItem, onError func(error)) *playbackProgressSession {
	r.mu.Lock()
	r.nextSession++
	id := r.nextSession
	r.mu.Unlock()
	return &playbackProgressSession{id: id, report: r, server: server, item: item, onError: onError}
}

func (r *playbackProgressReporter) run() {
	defer close(r.workerDone)
	for {
		r.mu.Lock()
		for len(r.queue) == 0 && !r.closed {
			r.wake.Wait()
		}
		if r.closed {
			r.mu.Unlock()
			return
		}
		report := r.queue[0]
		copy(r.queue, r.queue[1:])
		r.queue[len(r.queue)-1] = playbackProgressReport{}
		r.queue = r.queue[:len(r.queue)-1]
		if report.barrier {
			r.mu.Unlock()
			report.done <- nil
			continue
		}
		timeout := report.timeout
		if timeout <= 0 {
			timeout = playbackProgressTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		r.currentCancel = cancel
		r.currentDone = report.done
		r.mu.Unlock()

		err := report.server.UpdateProgressContext(ctx, report.item.ID, report.item.MediaID, report.position, report.duration)
		cancel()

		r.mu.Lock()
		r.currentCancel = nil
		r.currentDone = nil
		closed := r.closed
		r.mu.Unlock()
		if report.done != nil {
			completePlaybackProgress(report.done, err)
		} else if !closed && err != nil && report.onError != nil {
			report.onError(err)
		}
	}
}

func completePlaybackProgress(done chan error, err error) {
	select {
	case done <- err:
	default:
	}
}

func (s *playbackProgressSession) periodic(position, duration float64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.report == nil {
		return
	}
	s.report.enqueuePeriodic(playbackProgressReport{
		server: s.server, item: s.item, position: position, duration: duration,
		session: s.id, onError: s.onError,
	})
}

// finish atomically closes a playback session and queues its final position.
// Its request timeout starts when the worker sends it, not while it waits.
func (s *playbackProgressSession) finish(position, duration float64, timeout time.Duration) <-chan error {
	result := make(chan error, 1)
	if s == nil {
		result <- nil
		return result
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		result <- nil
		return result
	}
	s.closed = true
	queued := s.report.enqueueFinal(playbackProgressReport{
		server: s.server, item: s.item, position: position, duration: duration,
		timeout: timeout, session: s.id, done: result,
	})
	s.mu.Unlock()
	if !queued {
		result <- context.Canceled
	}
	return result
}

func (r *playbackProgressReporter) enqueuePeriodic(report playbackProgressReport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	for i := range r.queue {
		queued := &r.queue[i]
		if !queued.barrier && queued.done == nil && queued.session == report.session {
			*queued = report
			return
		}
	}
	r.queue = append(r.queue, report)
	r.wake.Signal()
}

func (r *playbackProgressReporter) enqueueFinal(report playbackProgressReport) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	// Once the session is closed, queued snapshots are stale; replacing them
	// with its final position also keeps shutdown from waiting on old updates.
	kept := r.queue[:0]
	for _, queued := range r.queue {
		if queued.session != report.session || queued.done != nil || queued.barrier {
			kept = append(kept, queued)
		}
	}
	r.queue = append(kept, report)
	r.wake.Signal()
	return true
}

func (r *playbackProgressReporter) flush() <-chan error {
	result := make(chan error, 1)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		result <- context.Canceled
		return result
	}
	r.queue = append(r.queue, playbackProgressReport{barrier: true, done: result})
	r.wake.Signal()
	r.mu.Unlock()
	return result
}

// close cancels any in-flight HTTP request and releases the worker before the
// application exits. The request uses a context, so net/http returns promptly.
func (r *playbackProgressReporter) close(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		for _, report := range r.queue {
			if report.done != nil {
				completePlaybackProgress(report.done, context.Canceled)
			}
		}
		r.queue = nil
		if r.currentDone != nil {
			completePlaybackProgress(r.currentDone, context.Canceled)
		}
		if r.currentCancel != nil {
			r.currentCancel()
		}
		r.wake.Broadcast()
	}
	r.mu.Unlock()
	select {
	case <-r.workerDone:
	case <-ctx.Done():
	}
}
