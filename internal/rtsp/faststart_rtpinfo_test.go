package rtsp

import (
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/headers"
	"github.com/pion/rtp"

	"github.com/VoltTech21/reostream/internal/baichuan"
)

func (f *fastStartStats) count() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.replays
}

// playFirstVideo runs DESCRIBE, SETUP and PLAY over TCP and returns the
// medias, the PLAY reply's RTP-Info and the first video packet received.
func playFirstVideo(t *testing.T, addr, path string) ([]*description.Media, headers.RTPInfo, *rtp.Packet) {
	t.Helper()
	u, err := base.ParseURL("rtsp://" + addr + "/" + path)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	tr := gortsplib.ProtocolTCP
	c := &gortsplib.Client{Scheme: u.Scheme, Host: u.Host, Protocol: &tr}
	if err := c.Start(); err != nil {
		t.Fatalf("client start: %v", err)
	}
	t.Cleanup(c.Close)
	desc, _, err := c.Describe(u)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if err := c.SetupAll(desc.BaseURL, desc.Medias); err != nil {
		t.Fatalf("setup: %v", err)
	}
	var once sync.Once
	first := make(chan *rtp.Packet, 1)
	c.OnPacketRTPAny(func(m *description.Media, _ format.Format, p *rtp.Packet) {
		if m.Type == description.MediaTypeVideo {
			once.Do(func() { first <- p.Clone() })
		}
	})
	res, err := c.Play(nil)
	if err != nil {
		t.Fatalf("play: %v", err)
	}
	var ri headers.RTPInfo
	if err := ri.Unmarshal(res.Header["RTP-Info"]); err != nil {
		t.Fatalf("RTP-Info %q: %v", res.Header["RTP-Info"], err)
	}
	select {
	case p := <-first:
		return desc.Medias, ri, p
	case <-time.After(5 * time.Second):
		t.Fatal("no video packet after PLAY")
	}
	return nil, nil, nil
}

// A reader sent the held pictures has to be told its stream starts at them.
//
// gortsplib built RTP-Info after OnReaderActivate had already queued the
// replay, from the live position: the next live sequence number and the
// wall clock now. The reader's first packets then carried earlier sequence
// numbers and timestamps than the start it was given; ffmpeg pins each
// media's clock to the RTP-Info rtptime, so the replayed keyframe came out
// of its demuxer with no timestamp and the frames after it ran from before
// zero. With bobby.4 (2026-11-21) Home Assistant's snapshots of every
// reostream camera timed out while WebRTC live view, which ignores
// RTP-Info, kept working.
func TestReplayRTPInfoNamesTheFirstReplayedPacket(t *testing.T) {
	if !fastStart.Load() {
		t.Skip("fast start turned off")
	}
	frames := captureFrames(t)
	srv, st := serverWithStream(t, "cam_main")
	feedUntilReady(t, st, frames)
	// A first reader, so the stream has a live position to be told about.
	playRTPInfo(t, srv.Addr(), "cam_main")
	feed(t, st, frames, len(frames))

	st.mu.Lock()
	held := len(st.gop)
	st.mu.Unlock()
	if held == 0 {
		t.Fatal("nothing held to replay; the fixture should leave a group of pictures")
	}
	before := st.fs.count()

	medias, ri, first := playFirstVideo(t, srv.Addr(), "cam_main")
	if n := st.fs.count(); n != before+1 {
		t.Fatalf("replays %d -> %d: the reader was not sent the held pictures", before, n)
	}
	vi := -1
	for i, m := range medias {
		if m.Type == description.MediaTypeVideo {
			vi = i
		}
	}
	e := ri[vi]
	if e.SequenceNumber == nil || e.Timestamp == nil {
		t.Fatalf("video RTP-Info %q has no seq or rtptime", ri.Marshal())
	}
	if *e.SequenceNumber != first.SequenceNumber {
		t.Fatalf("RTP-Info seq %d, first video packet seq %d", *e.SequenceNumber, first.SequenceNumber)
	}
	// The first packet sits at or just after the advertised start
	// (gortsplib names an instant 100 ms early to keep DTS <= PTS).
	if d := int32(first.Timestamp - *e.Timestamp); d < 0 || d > rtpClockHz/5 {
		t.Fatalf("first video packet is %d ticks after the RTP-Info rtptime; want 0..%d", d, rtpClockHz/5)
	}
	checkRTPInfoAgrees(t, medias, ri, time.Now())
}

// Turning fast start off drops the held pictures and stops the replay.
func TestSetFastStartOff(t *testing.T) {
	if !fastStart.Load() {
		t.Skip("fast start turned off")
	}
	defer SetFastStart(true)
	s := &Stream{}
	s.keepGOP(gopFrameOf(baichuan.FrameIFrame, 10), 1)
	SetFastStart(false)
	s.keepGOP(gopFrameOf(baichuan.FramePFrame, 10), 2)
	if len(s.gop) != 0 {
		t.Fatalf("still holding %d frames with fast start off", len(s.gop))
	}
	if FastStartEnabled() {
		t.Fatal("FastStartEnabled after SetFastStart(false)")
	}
}
