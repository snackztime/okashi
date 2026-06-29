package textarea

import (
	"strings"
	"testing"
)

// The wrap cache must grow with the buffer, not stay pinned at the MaxHeight
// default (99). Regression for the large-file render thrash. Note: CharLimit
// and MaxHeight are both 0 (okashi's settings) so the buffer isn't truncated;
// the resize must fire on the SetValue (load) path, not only on a keystroke.
func TestWrapCacheGrowsWithBuffer(t *testing.T) {
	m := New()
	m.CharLimit = 0 // unlimited (default 400 would truncate the test buffer)
	m.MaxHeight = 0 // okashi's setting
	m.SetWidth(72)
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("a distinct line number ")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteByte('\n')
	}
	m.SetValue(b.String()) // load path — no keystroke/Update
	if m.cache.Capacity() < 500 {
		t.Fatalf("cache capacity = %d, want >= 500 (buffer size) — cache thrashes on big files", m.cache.Capacity())
	}
}
