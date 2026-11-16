package lumon

import (
	"sync"
	"testing"
	"time"
)

// Joining reads the board while another refiner is stepping and refining it.
// Run under -race (make test) this fails if Join reads the board unlocked.
func TestJoiningABoardSomebodyIsWorkingDoesNotRace(t *testing.T) {
	rooms := NewRooms(7)
	rooms.EnsureOpen(1)
	room := rooms.Find(rooms.List()[0].Name)

	worker := NewSession(rooms, 1)
	worker.SendHomeAfter(time.Hour, time.Hour)
	worker.Name = "ONE"
	if !worker.Join(room) {
		t.Fatal("the first refiner could not join an empty board")
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			worker.UpdateEvent(FrameMsg(500 * time.Millisecond))
			worker.UpdateEvent(KeyMsg{Key: "enter", Runes: []rune{'\r'}})
			worker.UpdateEvent(KeyMsg{Key: "right"})
		}
	}()

	arriving := NewSession(rooms, 2)
	arriving.Name = "TWO"
	for i := 0; i < 300; i++ {
		arriving.Join(room)
		arriving.Leave()
		arriving.Watch(room)
		arriving.Leave()
	}
	close(stop)
	wg.Wait()
}
