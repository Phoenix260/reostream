package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VoltTech21/reostream/internal/hub"
	"github.com/VoltTech21/reostream/internal/rtsp"
)

type fakeFast struct{ on bool }

func (f *fakeFast) FastStartStatus() rtsp.FastStartStatus {
	return rtsp.FastStartStatus{Enabled: f.on, Streams: map[string]rtsp.FastStartStream{"cam_sub": {Replays: 3}}}
}
func (f *fakeFast) SetFastStart(on bool) { f.on = on }

// /api/status reports fast start, and POST /api/fast_start switches it,
// behind the stream login while /api/status stays open.
func TestFastStartAPI(t *testing.T) {
	ff := &fakeFast{on: true}
	s := New(StaticHubs(map[string]*hub.Hub{}))
	s.SetFastStarter(ff)
	h := RequireLogin(s.Handler(), "u", "p")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/status", nil))
	var body struct {
		FastStart *rtsp.FastStartStatus `json:"fast_start"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.FastStart == nil || !body.FastStart.Enabled {
		t.Fatalf("status fast_start = %+v, err %v, body %s", body.FastStart, err, rec.Body)
	}

	post := func(form string, auth bool) int {
		r := httptest.NewRequest("POST", "/api/fast_start", strings.NewReader(form))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if auth {
			r.SetBasicAuth("u", "p")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	if c := post("enabled=0", false); c != http.StatusUnauthorized || !ff.on {
		t.Fatalf("without login: %d, on=%v", c, ff.on)
	}
	if c := post("enabled=maybe", true); c != http.StatusBadRequest {
		t.Fatalf("bad value: %d", c)
	}
	if c := post("enabled=0", true); c != http.StatusOK || ff.on {
		t.Fatalf("off: %d, on=%v", c, ff.on)
	}
	if c := post("enabled=1", true); c != http.StatusOK || !ff.on {
		t.Fatalf("on: %d, on=%v", c, ff.on)
	}
}
