package lumon

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// What it costs to host a floor. BenchmarkRealFrame measures one session alone;
// here sessions run against each other, sharing rooms, the board locks (Update
// writes, View reads) and one heap. Figures are per session-frame, so whatever
// is above the single-session baseline is what sharing costs.

// The terminal every simulated refiner is on. Generous: a bigger
// screen is more cells to draw and more bytes to push, so this is the
// pessimistic case.
const (
	scaleTermW = 120
	scaleTermH = 40
)

// seatMany fills rooms the way the registry does and returns one model per
// simulated refiner. Assign fills a room before opening another, so n refiners
// share ceil(n/MaxDesks) boards and contend for their locks.
func seatMany(tb testing.TB, n int) []*Model {
	tb.Helper()
	rs := NewRooms(1)

	ms := make([]*Model, 0, n)
	for i := 0; i < n; i++ {
		m := JoinRoom(rs.Assign(), int64(i))
		if m == nil {
			tb.Fatalf("could not seat refiner %d of %d", i+1, n)
		}
		m.Resize(scaleTermW, scaleTermH)
		trueColour(m)
		ms = append(ms, m)
	}
	return ms
}

// runFloor drives every session for the given number of frames and returns the
// bytes drawn. Each worker is long-lived and runs without a barrier between
// frames, as a connection's goroutine does. A goroutine per session per frame
// joined on a WaitGroup would measure the scheduler: at 256 sessions over sixty
// per cent of its profile is pthread_cond_wait and pthread_cond_signal.
func runFloor(ms []*Model, frames int) int64 {
	workers := runtime.GOMAXPROCS(0)
	if workers > len(ms) {
		workers = len(ms)
	}

	var bytes int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			var n int64
			// Strided, so each worker's sessions are spread across rooms and contend for
			// their locks as connections do.
			for i := 0; i < frames; i++ {
				for j := w; j < len(ms); j += workers {
					m := ms[j]
					m.Update(FrameMsg(frameInterval))
					n += int64(len(m.View()))
				}
			}
			atomic.AddInt64(&bytes, n)
		}(w)
	}
	wg.Wait()
	return bytes
}

func benchFloor(b *testing.B, n int) {
	ms := seatMany(b, n)

	b.ReportAllocs()
	b.ResetTimer()
	bytes := runFloor(ms, b.N)
	b.StopTimer()

	frames := float64(b.N) * float64(n)
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/frames, "ns/session-frame")
	b.ReportMetric(float64(bytes)/frames, "bytes/session-frame")
	b.ReportMetric(float64(bytes)/frames*60/1024, "KB/s/session")
}

// benchLobby is a rush: everyone arriving at once, as when a link is posted
// somewhere. A lobby frame goes through Rooms.Directory, which takes the
// process-wide registry lock, sweeps idle rooms and asks every open room for
// its listing (another lock each), at least twice a frame per session. People
// choose a file from the lobby, so an arrival runs OpenNamed; nothing calls
// Assign.
func benchLobby(b *testing.B, n, rooms int) {
	rs := NewRooms(1)
	rs.EnsureOpen(rooms)

	ms := make([]*Model, 0, n)
	for i := 0; i < n; i++ {
		m := NewLocalSession(rs, int64(i), "REFINER")
		m.Resize(scaleTermW, scaleTermH)
		trueColour(m)
		ms = append(ms, m)
	}

	b.ReportAllocs()
	b.ResetTimer()
	runFloor(ms, b.N)
	b.StopTimer()

	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/(float64(b.N)*float64(n)),
		"ns/session-frame")
}

// One person waiting in the lobby, so the cost of the path itself is visible.
func BenchmarkLobby1(b *testing.B) { benchLobby(b, 1, DefaultRooms) }

// A rush. If the per-session cost climbs with the crowd, the registry lock is
// the bottleneck.
func BenchmarkLobby64(b *testing.B)  { benchLobby(b, 64, DefaultRooms) }
func BenchmarkLobby256(b *testing.B) { benchLobby(b, 256, DefaultRooms) }

// A busy floor. Directory walks every open room, so its cost grows with the
// room count, and a server with hundreds of refiners has scores of files open,
// not the four a quiet one keeps.
func BenchmarkLobby256Busy(b *testing.B) { benchLobby(b, 256, 64) }

// BenchmarkTakeASeat is one arrival committing to a file: what happens when
// somebody presses enter on a card.
func BenchmarkTakeASeat(b *testing.B) {
	rs := NewRooms(1)
	rs.EnsureOpen(DefaultRooms)
	names := rs.Directory()

	var seq atomic.Int64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := seq.Add(1)
			m := newView(i)
			m.Name = "REFINER"
			// A file already open, so this measures seating alone; BenchmarkOpenRoom
			// measures building a board.
			m.Join(rs.OpenNamed(names[int(i)%len(names)].Name))
			m.Leave()
		}
	})
}

// BenchmarkOpenRoom is what opening a file costs, the part of an arrival that
// can run long.
func BenchmarkOpenRoom(b *testing.B) {
	rs := NewRooms(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rs.mu.Lock()
		rs.open()
		rs.mu.Unlock()
	}
}

func BenchmarkFloor1(b *testing.B)    { benchFloor(b, 1) }
func BenchmarkFloor16(b *testing.B)   { benchFloor(b, 16) }
func BenchmarkFloor64(b *testing.B)   { benchFloor(b, 64) }
func BenchmarkFloorFull(b *testing.B) { benchFloor(b, MaxRooms*MaxDesks) }

// TestAFloorOfHundredsKeepsUp reports throughput and asserts only that the
// floor is servable at all. A test machine is not a server, so a tight
// wall-clock budget would fail on a loaded machine.
func TestAFloorOfHundredsKeepsUp(t *testing.T) {
	if testing.Short() {
		t.Skip("a full floor is not a short test")
	}
	const (
		n      = MaxRooms * MaxDesks
		frames = 60
	)
	ms := seatMany(t, n)

	start := time.Now()
	bytes := runFloor(ms, frames)
	elapsed := time.Since(start)

	// Throughput against demand: n refiners at sixty frames a second need n*60
	// session-frames a second, and the headroom is how much more the machine can do.
	done := float64(n) * float64(frames)
	achieved := done / elapsed.Seconds()
	needed := float64(n) * 60
	headroom := achieved / needed
	kbs := float64(bytes) / done * 60 / 1024

	t.Logf("%d refiners on %d cores: %.0f session-frames/s against %.0f needed "+
		"(%.1fx headroom), %.0fµs of CPU per session-frame, %.0f KB/s drawn per "+
		"session, %.1f MB/s across the floor",
		n, runtime.NumCPU(), achieved, needed, headroom,
		elapsed.Seconds()/done*1e6, kbs, kbs*n/1024)
	// achieved already counts every core, so the saturation point must not be
	// multiplied by them again.
	saturates := int(float64(n) * headroom)
	t.Logf("the floor saturates at about %d refiners, %d a core",
		saturates, saturates/runtime.GOMAXPROCS(0))

	// Under -race the floor still runs, since 256 sessions sharing 64 boards is
	// dense lock traffic for the detector, but the verdict is withheld: the
	// instrumentation cuts throughput several times over.
	if raceEnabled {
		t.Log("built with -race: the floor was exercised but its speed is not judged")
		return
	}
	if headroom < 1 {
		t.Errorf("%d refiners need %.0f session-frames a second and the machine "+
			"managed %.0f; the floor cannot be served at sixty frames a second",
			n, needed, achieved)
	}
}

// TestTheWireIsMostlyIdleWhenTheBoardIs measures what a session costs the
// network. Bubble Tea sends only the rows that changed, so if Drift pushed a
// digit across a cell boundary every frame, every row would be dirty and a
// session would cost a full frame sixty times a second, as the per-session
// bandwidth figure for a server assumes. hysteresis.go's dead band
// exists to prevent that.
func TestTheWireIsMostlyIdleWhenTheBoardIs(t *testing.T) {
	// Measured in each state a session is in, because they differ by two orders
	// of magnitude.
	for _, state := range []struct {
		name  string
		setup func(*Model)
		// the share of rows that may change per frame
		budget float64
	}{
		{"a board at rest", func(*Model) {}, 0.10},
		// Woken and given time to swell: magnifyFrequency is 0.22, so a cluster takes
		// most of half a minute to cross MinGlyphHeight and be drawn as glyphs.
		{"a cluster swollen", func(m *Model) {
			wakeOne(m)
			for i := 0; i < 60*35; i++ {
				tick(m, 1)
				drawTo(m)
			}
		}, 0.90},
		{"a diversion running", func(m *Model) {
			typeWord(m, theWord)
			m.Update(key("enter"))
			tick(m, 90)
		}, 1.01},
	} {
		m := ready()
		trueColour(m)
		tick(m, 180) // past the arrival, and settled
		state.setup(m)

		prev := rowsOf(m)
		var dirty, total, wire int
		const frames = 300
		for i := 0; i < frames; i++ {
			tick(m, 1)
			now := rowsOf(m)
			changed := false
			for y := range now {
				total++
				if now[y] != prev[y] {
					dirty++
					changed = true
				}
			}
			// What the renderer would write: nothing for a frame identical to the last,
			// otherwise the home sequence, one byte for every line the diff skips, and each
			// changed line in full with its newline. Bytes, not rows, are what a link
			// carries.
			if changed {
				wire += len("\x1b[H")
				for y := range now {
					if now[y] == prev[y] {
						wire++
						continue
					}
					wire += len(now[y]) + 2
				}
			}
			prev = now
		}

		share := float64(dirty) / float64(total)
		full := float64(len(prev))
		t.Logf("%-20s redraws %5.1f%% of its rows per frame (%.1f of %.0f), %5.1f KB/s at 60fps",
			state.name, share*100, share*full, full,
			float64(wire)/float64(frames)*60/1024)

		if share > state.budget {
			t.Errorf("%s changes %.0f%% of rows per frame, over the %.0f%% this was "+
				"written against; the line diff stops buying anything and a session "+
				"costs a whole frame of traffic every frame",
				state.name, share*100, state.budget*100)
		}
	}
}

// wakeOne agitates a cluster in the viewport and scrolls to it. The grid is a
// hundred cells square and a terminal shows about twenty; planGlyphs plans only
// cells on screen, and the first group (as benchModel wakes) is usually off
// screen, which would measure an idle board.
func wakeOne(m *Model) {
	l := m.Layout
	for _, grp := range m.Grid.Groups {
		id := grp.NumberIDs[0]
		gx, gy := id%GridSize, id/GridSize
		if gx < l.ViewportX || gx >= l.ViewportX+l.ViewportW ||
			gy < l.ViewportY || gy >= l.ViewportY+l.ViewportH {
			continue
		}
		m.CursorX, m.CursorY = gx, gy
		m.Activate(grp)
		return
	}
}

// rowsOf is the drawn frame split into rows, the unit Bubble Tea diffs.
func rowsOf(m *Model) []string {
	buf := drawTo(m)
	_, h := buf.Size()
	out := make([]string, h)
	for y := 0; y < h; y++ {
		out[y] = buf.Row(y)
	}
	return out
}

// Mirroring the bins spends wire: another refiner's digits cross the watcher's
// grid, rules and bin row, most of the screen, for the second or so a flight
// takes. It is bounded: it lasts one flight, happens only on a refine, and a
// board at rest stays at about one per cent in the state table above.
func TestAWitnessedRefinementCostsWire(t *testing.T) {
	_, a, b := twoInARoom(t)
	b.Resize(scaleTermW, scaleTermH)
	trueColour(b)

	grp := activateFirstVisibleGroup(a)
	if grp == nil {
		t.Skip("no bad group visible")
	}
	id := grp.NumberIDs[0]
	a.CursorX, a.CursorY = id%GridSize, id/GridSize
	a.SelectedBin = 0
	a.Update(key("enter"))

	prev := rowsOf(b)
	var dirty, total int
	const frames = 120 // two seconds, comfortably longer than a flight
	for i := 0; i < frames; i++ {
		tick(b, 1)
		now := rowsOf(b)
		for y := range now {
			total++
			if now[y] != prev[y] {
				dirty++
			}
		}
		prev = now
	}

	share := float64(dirty) / float64(total)
	t.Logf("watching a refinement redraws %.1f%% of the rows per frame over two "+
		"seconds (%.1f of %.0f)", share*100, share*float64(len(prev)), float64(len(prev)))

	if share > 0.40 {
		t.Errorf("a witnessed refinement redraws %.0f%% of rows per frame, past the "+
			"40%% this was written against; the flight is costing more of the screen "+
			"than a diversion does", share*100)
	}
	if share < 0.02 {
		t.Errorf("a witnessed refinement redraws only %.1f%% of rows; the digits are "+
			"not crossing this screen at all", share*100)
	}
}

// seatAndWatch fills rooms and then puts watchers behind them. A watcher draws
// the same frame and sends the same rows as a refiner, since the diff does not
// know about desks; they only skip refining and waking.
func seatAndWatch(tb testing.TB, refiners, watchers int) []*Model {
	tb.Helper()
	rs := NewRooms(1)

	ms := make([]*Model, 0, refiners+watchers)
	for i := 0; i < refiners; i++ {
		m := JoinRoom(rs.Assign(), int64(i))
		if m == nil {
			tb.Fatalf("could not seat refiner %d of %d", i+1, refiners)
		}
		m.Resize(scaleTermW, scaleTermH)
		trueColour(m)
		ms = append(ms, m)
	}

	// Behind the boards that are already full, where anybody would watch from.
	rooms := rs.List()
	for i := 0; i < watchers; i++ {
		var seated *Room
		for _, l := range rooms {
			if r := rs.Find(l.Name); r != nil && r.Seated() >= MaxDesks && r.Waiting() < MaxWatchers {
				seated = r
				break
			}
		}
		if seated == nil {
			tb.Fatalf("no full room left to watch after %d watchers", i)
		}
		w := newView(int64(10000 + i))
		w.Resize(scaleTermW, scaleTermH)
		trueColour(w)
		if !w.Watch(seated) {
			tb.Fatalf("could not watch with watcher %d", i+1)
		}
		ms = append(ms, w)
	}
	return ms
}

func benchWatched(b *testing.B, refiners, watchers int) {
	ms := seatAndWatch(b, refiners, watchers)

	b.ReportAllocs()
	b.ResetTimer()
	bytes := runFloor(ms, b.N)
	b.StopTimer()

	frames := float64(b.N) * float64(len(ms))
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/frames, "ns/session-frame")
	b.ReportMetric(float64(bytes)/frames*60/1024, "KB/s/session")
}

// 128 at desks with 128 behind them, against BenchmarkFloorFull's floor all seated.
// If a watcher were cheaper than a refiner these two would differ.
func BenchmarkFloorWithWatchers(b *testing.B) { benchWatched(b, 128, 128) }
