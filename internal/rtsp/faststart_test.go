package rtsp

import (
	"testing"

	"github.com/VoltTech21/reostream/internal/baichuan"
)

func gopFrameOf(kind baichuan.FrameKind, n int) baichuan.Frame {
	data := append([]byte{0, 0, 0, 1, 0x65}, make([]byte, n)...)
	return baichuan.Frame{Kind: kind, Data: data}
}

// The held group of pictures starts at a keyframe, starts over at the next
// one, and holds nothing before the first.
func TestKeepGOPStartsAtKeyframe(t *testing.T) {
	if !fastStart {
		t.Skip("fast start turned off")
	}
	s := &Stream{}
	s.keepGOP(gopFrameOf(baichuan.FramePFrame, 10), 1)
	if len(s.gop) != 0 {
		t.Fatalf("held %d frames before any keyframe", len(s.gop))
	}
	s.keepGOP(gopFrameOf(baichuan.FrameIFrame, 10), 2)
	s.keepGOP(gopFrameOf(baichuan.FramePFrame, 10), 3)
	s.keepGOP(gopFrameOf(baichuan.FramePFrame, 10), 4)
	if len(s.gop) != 3 || s.gop[0].ts90 != 2 || s.gop[2].ts90 != 4 {
		t.Fatalf("gop = %+v", s.gop)
	}
	s.keepGOP(gopFrameOf(baichuan.FrameIFrame, 10), 5)
	if len(s.gop) != 1 || s.gop[0].ts90 != 5 {
		t.Fatalf("keyframe did not start the gop over: %+v", s.gop)
	}
}

// A group of pictures past the cap is dropped until the next keyframe.
func TestKeepGOPCap(t *testing.T) {
	if !fastStart {
		t.Skip("fast start turned off")
	}
	s := &Stream{}
	s.keepGOP(gopFrameOf(baichuan.FrameIFrame, gopMaxBytes-100), 1)
	s.keepGOP(gopFrameOf(baichuan.FramePFrame, 200), 2)
	if len(s.gop) != 0 || !s.gopFull {
		t.Fatalf("over the cap: %d frames, full=%v", len(s.gop), s.gopFull)
	}
	s.keepGOP(gopFrameOf(baichuan.FramePFrame, 10), 3)
	if len(s.gop) != 0 {
		t.Fatalf("held a frame while full")
	}
	s.keepGOP(gopFrameOf(baichuan.FrameIFrame, 10), 4)
	if len(s.gop) != 1 || s.gopFull {
		t.Fatalf("keyframe did not reset the cap")
	}
}

// The held frames are copies, not views of the camera's buffer.
func TestKeepGOPCopies(t *testing.T) {
	if !fastStart {
		t.Skip("fast start turned off")
	}
	s := &Stream{}
	f := gopFrameOf(baichuan.FrameIFrame, 4)
	s.keepGOP(f, 1)
	f.Data[len(f.Data)-1] = 0xAA
	if s.gop[0].data[len(s.gop[0].data)-1] == 0xAA {
		t.Fatal("held frame shares the camera's buffer")
	}
}
