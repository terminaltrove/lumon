package lumon

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Init adapts the platform-neutral initial effects to Bubble Tea commands.
func (m *Model) Init() tea.Cmd { return m.bubbleTeaCommands(m.InitialEffects()) }

// Update adapts Bubble Tea input and effects to the shared game model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, m.bubbleTeaCommands(m.UpdateEvent(fromBubbleTea(msg)))
}

// handleMouse retains the native-package helper used by focused model tests.
func (m *Model) handleMouse(msg tea.MouseMsg) { m.handleMouseEvent(fromBubbleTeaMouse(msg)) }

func (m *Model) bubbleTeaCommands(effects []Effect) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(effects))
	for _, effect := range effects {
		switch effect.Kind {
		case EffectFrame:
			cmds = append(cmds, frameCmd(effect.Since))
		case EffectLink:
			if cmd := m.linkCmd(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		case EffectQuit:
			cmds = append(cmds, tea.Quit)
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) linkCmd() tea.Cmd {
	ch := m.links
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		d, ok := <-ch
		if !ok {
			return nil
		}
		return LinkMsg(d)
	}
}

// frameCmd asks for the next frame and reports the actual elapsed time.
func frameCmd(since time.Time) tea.Cmd {
	return tea.Tick(frameRate, func(t time.Time) tea.Msg {
		if since.IsZero() {
			return FrameMsg(frameRate)
		}
		if d := t.Sub(since); d > 0 {
			return FrameMsg(d)
		}
		return FrameMsg(frameRate)
	})
}

func fromBubbleTea(msg tea.Msg) any {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return WindowSizeMsg{Width: msg.Width, Height: msg.Height}
	case tea.KeyMsg:
		return KeyMsg{Key: msg.String(), Runes: msg.Runes, Alt: msg.Alt}
	case tea.MouseMsg:
		return fromBubbleTeaMouse(msg)
	default:
		return msg
	}
}

func fromBubbleTeaMouse(msg tea.MouseMsg) MouseMsg {
	button := MouseNone
	switch msg.Button {
	case tea.MouseButtonLeft:
		button = MouseLeft
	case tea.MouseButtonWheelUp:
		button = MouseWheelUp
	case tea.MouseButtonWheelDown:
		button = MouseWheelDown
	}
	action := MouseMotion
	switch msg.Action {
	case tea.MouseActionPress:
		action = MousePress
	case tea.MouseActionRelease:
		action = MouseRelease
	}
	return MouseMsg{X: msg.X, Y: msg.Y, Button: button, Action: action}
}
