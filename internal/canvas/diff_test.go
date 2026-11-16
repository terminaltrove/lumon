package canvas

import (
	"strings"
	"testing"
)

func TestFrameDifferReplacesOnlyChangedRows(t *testing.T) {
	var d FrameDiffer
	first := d.Render("header\nold\nfooter")
	if !strings.Contains(first, "\x1b[2J") || !strings.Contains(first, "\x1b[3;1H") {
		t.Fatalf("initial frame must clear and paint every row: %q", first)
	}
	if got := d.Render("header\nold\nfooter"); got != "" {
		t.Fatalf("unchanged frame should produce no terminal work: %q", got)
	}
	got := d.Render("header\nnew\nfooter")
	if got != "\x1b[2;1H\x1b[0mnew\x1b[0m\x1b[K" {
		t.Fatalf("changed row was not replaced independently: %q", got)
	}
}

func TestFrameDifferClearsRemovedRowsAndRepaintsAfterResize(t *testing.T) {
	var d FrameDiffer
	d.Render("first\nsecond\nthird")
	got := d.Render("first")
	if !strings.Contains(got, "\x1b[2;1H\x1b[0m\x1b[0m\x1b[K") ||
		!strings.Contains(got, "\x1b[3;1H\x1b[0m\x1b[0m\x1b[K") {
		t.Fatalf("removed rows left stale content: %q", got)
	}
	d.Reset()
	if got := d.Render("first"); !strings.Contains(got, "\x1b[2J") || !strings.Contains(got, "first") {
		t.Fatalf("resize needs a complete repaint: %q", got)
	}
}

// Checked frame after frame, including the first and a resize.
func TestRenderBufferSendsWhatRenderSends(t *testing.T) {
	var cells, text FrameDiffer
	st := (*Styler)(nil)
	frames := []func(b *Buffer){
		func(b *Buffer) { b.Text(0, 0, "header", Attr{FG: RGB(0, 255, 255)}); b.Text(0, 1, "old", Attr{}) },
		func(b *Buffer) { b.Text(0, 0, "header", Attr{FG: RGB(0, 255, 255)}); b.Text(0, 1, "old", Attr{}) },
		func(b *Buffer) {
			b.Text(0, 0, "header", Attr{FG: RGB(0, 255, 255)})
			b.Text(0, 1, "new", Attr{Bold: true})
		},
		func(b *Buffer) { b.Text(0, 0, "header", Attr{FG: RGB(1, 255, 255)}) },
	}
	b := NewBuffer(12, 3)
	for i, draw := range frames {
		b = b.Reuse(12, 3, Attr{})
		draw(b)
		if got, want := cells.RenderBuffer(b, st), text.Render(b.StringWith(st)); got != want {
			t.Fatalf("frame %d: cells sent %q, text sent %q", i, got, want)
		}
	}
	b = b.Reuse(10, 2, Attr{})
	b.Text(0, 0, "resized", Attr{})
	text.Reset()
	if got, want := cells.RenderBuffer(b, st), text.Render(b.StringWith(st)); got != want {
		t.Fatalf("after a resize: cells sent %q, text sent %q", got, want)
	}
}
