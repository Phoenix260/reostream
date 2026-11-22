package rtsp

import (
	"sort"
	"sync"
	"time"
)

// fastStartStats records what the keyframe replay did for one stream's
// readers. It is what lets a fast-start problem be told apart from a camera
// problem without a shell on the host: GET /api/status carries it.
type fastStartStats struct {
	mu         sync.Mutex
	replays    uint64
	skips      uint64
	lastAt     time.Time
	lastFrames int
	lastPkts   int
	lastSpan   time.Duration
	lastSkip   string
	lastSkipAt time.Time
}

func (f *fastStartStats) replayed(frames, pkts int, first, last time.Time) {
	f.mu.Lock()
	f.replays++
	f.lastAt = time.Now()
	f.lastFrames, f.lastPkts = frames, pkts
	f.lastSpan = last.Sub(first)
	f.mu.Unlock()
}

func (f *fastStartStats) skip(why string) {
	f.mu.Lock()
	f.skips++
	f.lastSkip, f.lastSkipAt = why, time.Now()
	f.mu.Unlock()
}

// FastStartStream is one stream's entry in FastStartStatus.
type FastStartStream struct {
	// Replays counts readers that were sent the held pictures.
	Replays uint64 `json:"replays"`
	// Skips counts readers that started without them, and LastSkip says why.
	Skips    uint64 `json:"skips"`
	LastSkip string `json:"last_skip,omitempty"`
	// HeldFrames is the size of the group of pictures held right now.
	HeldFrames int `json:"held_frames"`
	// The last replay: when, how much, and the time it covered once
	// squeezed (frames sit 1 ms apart, so this is about frames-1 ms).
	LastReplayAgeSeconds *float64 `json:"last_replay_age_seconds,omitempty"`
	LastReplayFrames     int      `json:"last_replay_frames,omitempty"`
	LastReplayPackets    int      `json:"last_replay_packets,omitempty"`
	LastReplaySpanMillis int64    `json:"last_replay_span_ms,omitempty"`
}

// FastStartStatus is the fast_start object of GET /api/status.
type FastStartStatus struct {
	Enabled bool                       `json:"enabled"`
	Streams map[string]FastStartStream `json:"streams"`
}

// FastStartStatus reports the replay switch and each RTSP path's counters.
func (s *Server) FastStartStatus() FastStartStatus {
	s.mu.RLock()
	paths := make([]string, 0, len(s.streams))
	for p := range s.streams {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	streams := make([]*Stream, len(paths))
	for i, p := range paths {
		streams[i] = s.streams[p]
	}
	s.mu.RUnlock()

	out := FastStartStatus{Enabled: fastStart.Load(), Streams: make(map[string]FastStartStream, len(paths))}
	for i, st := range streams {
		st.mu.Lock()
		held := len(st.gop)
		st.mu.Unlock()
		f := &st.fs
		f.mu.Lock()
		e := FastStartStream{
			Replays:              f.replays,
			Skips:                f.skips,
			LastSkip:             f.lastSkip,
			HeldFrames:           held,
			LastReplayFrames:     f.lastFrames,
			LastReplayPackets:    f.lastPkts,
			LastReplaySpanMillis: f.lastSpan.Milliseconds(),
		}
		if !f.lastAt.IsZero() {
			age := time.Since(f.lastAt).Seconds()
			e.LastReplayAgeSeconds = &age
		}
		f.mu.Unlock()
		out.Streams[paths[i]] = e
	}
	return out
}

// SetFastStart is SetFastStart for callers that hold a Server.
func (s *Server) SetFastStart(on bool) { SetFastStart(on) }
