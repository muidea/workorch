package tui

import "github.com/charmbracelet/bubbles/viewport"

func newMainViewport() *viewport.Model {
	vp := viewport.New(0, 0)
	vp.MouseWheelEnabled = false
	return &vp
}

func (m *AppModel) ensureMainViewport() *viewport.Model {
	if m.mainViewport == nil {
		m.mainViewport = newMainViewport()
	}
	m.mainViewport.MouseWheelEnabled = false
	return m.mainViewport
}

func (m *AppModel) resetMainViewport() {
	m.ensureMainViewport().GotoTop()
}

func (m *AppModel) scrollMain(delta int) {
	vp := m.ensureMainViewport()
	if delta < 0 {
		vp.ScrollUp(-delta)
		return
	}
	vp.ScrollDown(delta)
}
