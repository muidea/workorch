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
	m.mainViewportUserScrolled = false
	m.mainViewportPendingUpdate = false
	m.ensureMainViewport().GotoTop()
}

func (m *AppModel) followMainViewportTail() {
	m.mainViewportUserScrolled = false
	m.mainViewportPendingUpdate = false
	width, height := m.contentDimensions()
	if width > 0 && height > 0 {
		m.refreshMainViewport(width, height)
	}
	m.ensureMainViewport().GotoBottom()
}

func (m *AppModel) followMainViewportTailUnlessUserScrolled() {
	if m.mainViewportUserScrolled {
		m.mainViewportPendingUpdate = true
		return
	}
	m.followMainViewportTail()
}

func (m *AppModel) scrollMain(delta int) {
	vp := m.ensureMainViewport()
	if delta < 0 {
		vp.ScrollUp(-delta)
	} else {
		vp.ScrollDown(delta)
	}
	m.mainViewportUserScrolled = true
	m.mainViewportPendingUpdate = false
}
