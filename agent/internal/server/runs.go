package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

const (
	// turnTimeout bounds one chat turn, however long it waits on builds or approvals.
	turnTimeout = 2 * time.Hour
	// A reconnecting client is replayed what it missed, up to these limits.
	replayMaxFrames = 3000
	replayMaxBytes  = 8 << 20
)

// subscriber is one connected client watching a run.
type subscriber struct {
	send func([]byte) error
}

// run is one chat turn executing in the background. It belongs to the chat, not to the WebSocket
// that started it: closing the UI, switching tabs or losing the phone's connection does not stop
// it, and a client that connects later is replayed what it missed and then follows it live.
type run struct {
	key    string
	cancel context.CancelFunc

	mu        sync.Mutex
	frames    [][]byte
	bytes     int
	subs      map[*subscriber]struct{}
	stopped   bool // the user pressed stop
	shutdown  bool // the agent is shutting down
	persisted bool // the final reply is in the chat store
}

// emit records a frame for replay and delivers it to every attached client.
func (r *run) emit(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	r.mu.Lock()
	r.frames = append(r.frames, b)
	r.bytes += len(b)
	for len(r.frames) > 1 && (len(r.frames) > replayMaxFrames || r.bytes > replayMaxBytes) {
		r.bytes -= len(r.frames[0])
		r.frames = r.frames[1:]
	}
	subs := make([]*subscriber, 0, len(r.subs))
	for s := range r.subs {
		subs = append(subs, s)
	}
	r.mu.Unlock()
	for _, s := range subs {
		if s.send(b) != nil {
			r.detach(s)
		}
	}
}

// attach replays everything the run has emitted to s, then delivers new frames to it.
func (r *run) attach(s *subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range r.frames {
		if s.send(f) != nil {
			return
		}
	}
	if r.subs == nil {
		r.subs = map[*subscriber]struct{}{}
	}
	r.subs[s] = struct{}{}
}

func (r *run) detach(s *subscriber) {
	r.mu.Lock()
	delete(r.subs, s)
	r.mu.Unlock()
}

func (r *run) isPersisted() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.persisted
}

// runs tracks the turns in flight, one per chat.
type runs struct {
	mu sync.Mutex
	m  map[string]*run
}

func newRuns() *runs { return &runs{m: map[string]*run{}} }

func (rs *runs) get(key string) *run {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.m[key]
}

// begin registers a new run for key. It returns false when one is already in flight.
func (rs *runs) begin(key string) (*run, context.Context, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.m[key] != nil {
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	r := &run{key: key, cancel: cancel}
	rs.m[key] = r
	return r, ctx, true
}

func (rs *runs) end(r *run) {
	rs.mu.Lock()
	if rs.m[r.key] == r {
		delete(rs.m, r.key)
	}
	rs.mu.Unlock()
	r.cancel()
}

// stop cancels the chat's run at the user's request.
func (rs *runs) stop(key string) bool {
	r := rs.get(key)
	if r == nil {
		return false
	}
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	r.cancel()
	return true
}

// stopAll cancels every run because the agent is shutting down.
func (rs *runs) stopAll() {
	rs.mu.Lock()
	all := make([]*run, 0, len(rs.m))
	for _, r := range rs.m {
		all = append(all, r)
	}
	rs.mu.Unlock()
	for _, r := range all {
		r.mu.Lock()
		r.shutdown = true
		r.mu.Unlock()
		r.cancel()
	}
}

// detachEverywhere removes a disconnected client from whatever it was watching.
func (rs *runs) detachEverywhere(s *subscriber) {
	rs.mu.Lock()
	all := make([]*run, 0, len(rs.m))
	for _, r := range rs.m {
		all = append(all, r)
	}
	rs.mu.Unlock()
	for _, r := range all {
		r.detach(s)
	}
}
