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
	m.mainViewportUnreadCount = 0
	m.setActiveWorkspaceFollowTail(true)
	m.ensureMainViewport().GotoTop()
}

func (m *AppModel) followMainViewportTail() {
	m.mainViewportUserScrolled = false
	m.mainViewportPendingUpdate = false
	m.mainViewportUnreadCount = 0
	m.setActiveWorkspaceFollowTail(true)
	width, height := m.contentDimensions()
	if width > 0 && height > 0 {
		m.refreshMainViewport(width, height)
	}
	m.ensureMainViewport().GotoBottom()
}

func (m *AppModel) followMainViewportTailUnlessUserScrolled() {
	if m.mainViewportUserScrolled || !m.activeWorkspaceFollowTail() {
		m.mainViewportPendingUpdate = true
		m.mainViewportUnreadCount++
		if entry := m.workspaceRuntimeState(m.activeWorkspaceID); entry != nil {
			entry.UnreadUpdateCount = m.mainViewportUnreadCount
			entry.FollowTail = false
		}
		return
	}
	m.followMainViewportTail()
}

func (m *AppModel) scrollMain(delta int) {
	vp := m.ensureMainViewport()
	if delta < 0 {
		vp.ScrollUp(-delta)
		m.mainViewportUserScrolled = true
		m.mainViewportPendingUpdate = false
		m.setActiveWorkspaceFollowTail(false)
		return
	}
	vp.ScrollDown(delta)
	if vp.AtBottom() {
		m.mainViewportUserScrolled = false
		m.mainViewportPendingUpdate = false
		m.mainViewportUnreadCount = 0
		m.setActiveWorkspaceFollowTail(true)
		return
	}
	m.mainViewportUserScrolled = true
	m.mainViewportPendingUpdate = false
	m.setActiveWorkspaceFollowTail(false)
}

func (m *AppModel) activeWorkspaceFollowTail() bool {
	entry := m.workspaceRuntimeState(m.activeWorkspaceID)
	if entry == nil {
		return !m.mainViewportUserScrolled
	}
	return entry.FollowTail && !m.mainViewportUserScrolled
}

func (m *AppModel) setActiveWorkspaceFollowTail(follow bool) {
	entry := m.ensureWorkspaceRuntimeState(m.activeWorkspaceID)
	if entry == nil {
		return
	}
	entry.FollowTail = follow
	if follow {
		entry.UnreadUpdateCount = 0
	}
}
