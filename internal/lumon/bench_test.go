package lumon

import (
	"github.com/terminaltrove/lumon/internal/noise"
	"github.com/terminaltrove/lumon/internal/theme"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/terminaltrove/lumon/internal/canvas"
)

// trueColour gives a model a styler that emits colour. Without it lipgloss
// detects a non-terminal writer, decides Ascii and strips every escape, so the
// benchmark measures runes only. cmd/lumon's rendererFor calls SetColorProfile
// for the same reason.
func trueColour(m *Model) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	m.UseRenderer(r)
}

func benchModel(b *testing.B) *Model {
	b.Helper()
	m := NewModel(1)
	m.Resize(120, 40)
	// Agitate a cluster so the physics path is exercised.
	for _, grp := range m.Grid.Groups {
		if grp.NumberIDs[0]/GridSize < 20 {
			m.Activate(grp)
			break
		}
	}
	return m
}

// One frame of simulation plus one of rendering must fit comfortably inside
// 16.6ms.
func BenchmarkFrame(b *testing.B) {
	m := benchModel(b)
	buf := canvas.NewBuffer(120, 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		buf.Clear()
		m.Draw(buf)
		_ = buf.String()
	}
}

// The board seen as a volume: two thousand-odd digits of mass through the
// camera on top of the two hundred the flat board draws. It may cost more, but
// not enough to make the tour stutter.
func BenchmarkSolidFrame(b *testing.B) {
	m := benchModel(b)
	m.solid = NewSolid(m)
	buf := canvas.NewBuffer(120, 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		buf.Clear()
		m.Draw(buf)
		_ = buf.String()
	}
}

func BenchmarkSimulationStepOnly(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
	}
}

func BenchmarkDrawOnly(b *testing.B) {
	m := benchModel(b)
	buf := canvas.NewBuffer(120, 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Clear()
		m.Draw(buf)
		_ = buf.String()
	}
}

func BenchmarkIdleMarkFrame(b *testing.B) {
	m := benchModel(b)
	m.StartScreensaver()
	buf := canvas.NewBuffer(120, 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		buf.Clear()
		m.Draw(buf)
		_ = buf.String()
	}
}

func BenchmarkNewGrid(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewGrid(1)
	}
}

func BenchmarkDriftStep(b *testing.B) {
	l := NewLayout(120, 40)
	d := NewDrift()
	d.Sync(l)
	b.Logf("viewport %dx%d = %d cells, pool %d", l.ViewportW, l.ViewportH, l.ViewportW*l.ViewportH, d.Len())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Step(1.0 / 60)
	}
}

func BenchmarkPerlinNoise3D(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = noise.PerlinAnimation.Noise3D(float64(i%97)*0.18, float64(i%53)*0.18, float64(i)*0.001)
	}
}

// The frame as View renders it, and its size on the wire. BenchmarkFrame calls
// String where View calls StringWith(m.styler), so it misses the Styler cache
// and always takes the unstyled fast path. Remotely the frame is the traffic:
// Bubble Tea diffs it line by line, and drift touches most rows, so most of
// this goes to every seated refiner sixty times a second.
func BenchmarkRealFrame(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	b.ReportAllocs()
	b.ResetTimer()

	n := 0
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		n = len(m.View())
	}
	b.ReportMetric(float64(n), "bytes/frame")
	b.ReportMetric(float64(n)*60/1024, "KB/s@60fps")
}

// Three computer refiners at work: their lenses, plates and flights on this
// screen, and their stepping on the board.
func BenchmarkRealFrameWithCompany(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	m.AllowBots(3)
	tick(m, 10*60) // until they are refining
	b.ReportAllocs()
	b.ResetTimer()

	n := 0
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		n = len(m.View())
	}
	b.ReportMetric(float64(n), "bytes/frame")
}

// On a theme that names a Paper, every blank cell has a background, so no run
// is the zero attribute and StringWith's fast path cannot help.
func BenchmarkRealFramePaperTheme(b *testing.B) {
	m := benchModel(b)
	for _, t := range theme.Themes() {
		if t.Paper.Set() {
			m.Theme = t
			break
		}
	}
	if m.Theme.Paper.Set() == false {
		b.Skip("no theme names a paper")
	}
	trueColour(m)

	b.ReportAllocs()
	b.ResetTimer()

	n := 0
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		n = len(m.View())
	}
	b.ReportMetric(float64(n), "bytes/frame")
	b.ReportMetric(float64(n)*60/1024, "KB/s@60fps")
}

// benchDrawer is the lobby drawer, the first screen everybody sees. People sit
// still on it, so a frame that costs the same whether or not anything moved
// wastes bandwidth on every seat.
func benchDrawer(b *testing.B) *Model {
	b.Helper()
	rs := NewRooms(17)
	rs.EnsureOpen(DefaultRooms)
	m := NewLocalSession(rs, 1, "mark")
	m.Resize(120, 40)
	for i := 0; i < 200; i++ { // let the fade finish and the reel settle
		m.Update(FrameMsg(frameInterval))
	}
	trueColour(m)
	return m
}

// wireBytes reports the bytes in lines that changed since prev, the lines
// Bubble Tea repaints. An idle drawer produces identical frames, so the full
// frame size would overstate its traffic. The board quotes the full frame
// because drift changes most rows.
func wireBytes(m *Model, prev []string) ([]string, int) {
	cur := strings.Split(m.View(), "\n")
	n := 0
	for i := range cur {
		if i >= len(prev) || cur[i] != prev[i] {
			n += len(cur[i])
		}
	}
	return cur, n
}

// At rest, which is most of the time anyone spends here.
func BenchmarkDrawerFrame(b *testing.B) {
	m := benchDrawer(b)
	prev := strings.Split(m.View(), "\n")
	b.ReportAllocs()
	b.ResetTimer()

	total := 0
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		var n int
		prev, n = wireBytes(m, prev)
		total += n
	}
	per := float64(total) / float64(b.N)
	b.ReportMetric(per, "wire-bytes/frame")
	b.ReportMetric(per*60/1024, "KB/s@60fps")
}

// Turning flat out: the card is redrawn at a new height every frame, so every
// row of it changes.
func BenchmarkDrawerTurningFrame(b *testing.B) {
	m := benchDrawer(b)
	prev := strings.Split(m.View(), "\n")
	b.ReportAllocs()
	b.ResetTimer()

	total := 0
	for i := 0; i < b.N; i++ {
		if !m.rolodex().Turning() {
			m.rolodex().Turn(1)
		}
		m.Update(FrameMsg(frameInterval))
		var n int
		prev, n = wireBytes(m, prev)
		total += n
	}
	per := float64(total) / float64(b.N)
	b.ReportMetric(per, "wire-bytes/frame")
	b.ReportMetric(per*60/1024, "KB/s@60fps")
}

// The most expensive diversion. SPH sums a kernel over every neighbour of every
// particle, four substeps a frame, inside a 16.6ms budget shared with the
// chrome.
func BenchmarkFountainFrame(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	m.fountain = NewFountain(m, m.gridPanel().Play(), m.viewRng)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.fountain.Step(frameInterval.Seconds())
		_ = m.View()
	}
}

// One ray per column, marching cell to cell, so the cost is the cells crossed
// and not the resolution. It should be much cheaper than the fluid.
func BenchmarkCorridorFrame(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	m.halls = NewCorridors(m, m.gridPanel().Play(), m.viewRng)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.halls.Step(frameInterval.Seconds())
		_ = m.View()
	}
}

// Sparks do not interact, so this is O(n) with no neighbour queries, and should
// be much cheaper than the fluid at a comparable particle count.
func BenchmarkFireworksFrame(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	m.sky = NewFireworks(m, m.gridPanel().Play(), m.viewRng)
	// Far enough in that the sky is full.
	for i := 0; i < 60*20; i++ {
		m.sky.Step(frameInterval.Seconds(), m.BinFill)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.sky.Step(frameInterval.Seconds(), m.BinFill)
		_ = m.View()
	}
}

// Two hundred and sixty characters on the panel at once, one cell and one
// buf.Set apiece, with nothing rasterised or cached.
func BenchmarkStatementFrame(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	m.comply = NewStatement(m)
	m.comply.SetLevel(1)
	for i := 0; i < 120; i++ {
		m.comply.Resize(m.gridPanel().Play())
		m.comply.Step(frameInterval.Seconds())
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.comply.Step(frameInterval.Seconds())
		_ = m.View()
	}
}

// The steady state must be a placement, not a payload. The mark shares rows
// with the file bar, which Bubble Tea repaints often; if the image were sent on
// each repaint this would be several times BenchmarkRealFrame. The transmit is
// on the first frame and averages away over b.N.
func BenchmarkRealFrameKitty(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	m.UseGraphics(GraphicsKitty)
	m.View() // the one transmit, out of the way

	b.ReportAllocs()
	b.ResetTimer()

	n := 0
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		n = len(m.View())
	}
	b.ReportMetric(float64(n), "bytes/frame")
	b.ReportMetric(float64(n)*60/1024, "KB/s@60fps")
}

// The inline protocol puts about 9.7KB in every View string, so this measures
// the bytes in lines that changed, as the drawer benchmarks do. The mark's rows
// do not change between frames, so Bubble Tea sends the picture once; on the
// wire it costs about what kitty does, and the extra is memory only.
func BenchmarkRealFrameITerm(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	m.UseGraphics(GraphicsITerm)
	prev := strings.Split(m.View(), "\n") // the one that carries the picture

	b.ReportAllocs()
	b.ResetTimer()

	total := 0
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		var n int
		prev, n = wireBytes(m, prev)
		total += n
	}
	per := float64(total) / float64(b.N)
	b.ReportMetric(per, "wire-bytes/frame")
	b.ReportMetric(per*60/1024, "KB/s@60fps")
}

// The same measurement for kitty, to compare with the iTerm one.
func BenchmarkRealFrameKittyWire(b *testing.B) {
	m := benchModel(b)
	trueColour(m)
	m.UseGraphics(GraphicsKitty)
	prev := strings.Split(m.View(), "\n")

	b.ReportAllocs()
	b.ResetTimer()

	total := 0
	for i := 0; i < b.N; i++ {
		m.Update(FrameMsg(frameInterval))
		var n int
		prev, n = wireBytes(m, prev)
		total += n
	}
	per := float64(total) / float64(b.N)
	b.ReportMetric(per, "wire-bytes/frame")
	b.ReportMetric(per*60/1024, "KB/s@60fps")
}
