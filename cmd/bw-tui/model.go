package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jallum/beadwork/internal/issue"
)

// overlayKind identifies the active overlay prompt.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayConfirmClose
	overlayConfirmReopen
	overlayCommentInput
)

const refreshInterval = 3 * time.Second

type tickMsg struct{}

// Status filter options, cycled with 's'.
var statusFilters = []string{"", "open", "in_progress", "closed", "deferred"}

// viewKind identifies which top-level view is active.
type viewKind int

const (
	viewList viewKind = iota
	viewKanban
	viewTree
)

// focus tracks which pane has keyboard focus.
type focus int

const (
	focusList focus = iota
	focusDetail
)

// issueItem wraps an issue for the list.Model.
type issueItem struct {
	issue        *issue.Issue
	repoName     string // empty when single-repo
	openBlockers int    // count of non-closed blockers
}

func (i issueItem) Title() string {
	var b strings.Builder

	blocked := i.openBlockers > 0 && i.issue.Status != "closed"
	b.WriteString(styledStatusIcon(i.issue.Status, blocked))
	b.WriteString(" ")
	if i.repoName != "" {
		b.WriteString(styledRepoName(i.repoName))
		b.WriteString(" ")
	}
	b.WriteString(styledID(i.issue.ID))
	b.WriteString(" ")
	b.WriteString(styledPriorityBadge(i.issue.Priority))
	b.WriteString("  ")
	b.WriteString(i.issue.Title)

	return b.String()
}

func (i issueItem) Description() string {
	var parts []string
	if i.issue.Type != "" {
		parts = append(parts, strings.ToUpper(i.issue.Type))
	}
	if i.issue.Assignee != "" {
		parts = append(parts, "→ "+i.issue.Assignee)
	}
	if i.openBlockers > 0 {
		parts = append(parts, fmt.Sprintf("blocked by %d", i.openBlockers))
	}
	if len(i.issue.Labels) > 0 {
		parts = append(parts, strings.Join(i.issue.Labels, ", "))
	}
	return strings.Join(parts, " · ")
}

func (i issueItem) FilterValue() string {
	return i.issue.ID + " " + i.issue.Title
}

type model struct {
	repos     []*RepoSource
	view      viewKind
	list      list.Model
	kanban    kanbanData
	tree      treeData
	detail    *issue.Issue
	viewport  viewport.Model
	focus     focus
	width     int
	height    int
	showHelp  bool
	statusIdx int // index into statusFilters

	// Overlay state
	overlay    overlayKind
	overlayID  string          // issue ID the overlay applies to
	textInput  textinput.Model
	statusMsg  string          // transient status message from last action
}

func newModel(repos []*RepoSource) model {
	ti := textinput.New()
	ti.Placeholder = "Enter comment..."
	ti.CharLimit = 500

	m := model{
		repos:     repos,
		viewport:  viewport.New(),
		textInput: ti,
	}
	m.list = m.buildList()
	return m
}

func (m *model) buildList() list.Model {
	multiRepo := len(m.repos) > 1
	statusFilter := statusFilters[m.statusIdx]

	var items []list.Item
	for _, r := range m.repos {
		filter := issue.Filter{}
		if statusFilter != "" {
			filter.Status = statusFilter
		}
		issues, err := r.Store.List(filter)
		if err != nil {
			continue
		}
		for _, iss := range issues {
			name := ""
			if multiRepo {
				name = r.Name
			}
			openBlockers := 0
			for _, bid := range iss.BlockedBy {
				if !r.Store.IsClosed(bid) {
					openBlockers++
				}
			}
			items = append(items, issueItem{
				issue:        iss,
				repoName:     name,
				openBlockers: openBlockers,
			})
		}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, m.width, m.height)

	title := "Beadwork"
	if statusFilter != "" {
		title += " [" + statusFilter + "]"
	}
	l.Title = title
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	return l
}

func (m model) Init() tea.Cmd {
	// Record initial hashes so first tick doesn't trigger a spurious rebuild.
	for _, r := range m.repos {
		r.Changed()
	}
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tickMsg:
		changed := false
		for _, r := range m.repos {
			if r.Changed() {
				changed = true
			}
		}
		if changed {
			switch m.view {
			case viewList:
				m.list = m.buildList()
			case viewKanban:
				m.kanban = newKanbanData(m.repos)
			case viewTree:
				m.tree = newTreeData(m.repos)
			}
			if m.detail != nil {
				for _, r := range m.repos {
					if iss, err := r.Store.Get(m.detail.ID); err == nil {
						m.detail = iss
						m.viewport.SetContent(m.renderDetailContent(iss))
						break
					}
				}
			}
		}
		return m, tickCmd()

	case tea.KeyMsg:
		// Handle overlay input first
		if m.overlay != overlayNone {
			return m.updateOverlay(msg)
		}

		// Don't intercept keys while filtering (list view only)
		if m.view == viewList && m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q"))):
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("?"))):
			m.showHelp = !m.showHelp
			return m, nil

		// View switching
		case key.Matches(msg, key.NewBinding(key.WithKeys("1"))):
			switchView(&m, viewList)
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("2"))):
			switchView(&m, viewKanban)
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("3"))):
			switchView(&m, viewTree)
			return m, nil

		// Mutations (work on selected issue in any view)
		case key.Matches(msg, key.NewBinding(key.WithKeys("S"))):
			if iss := m.selectedIssue(); iss != nil {
				result := doStart(m.repos, iss.ID)
				m.statusMsg = result.String()
				m.refreshCurrentView()
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("X"))):
			if iss := m.selectedIssue(); iss != nil {
				m.overlay = overlayConfirmClose
				m.overlayID = iss.ID
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("O"))):
			if iss := m.selectedIssue(); iss != nil {
				m.overlay = overlayConfirmReopen
				m.overlayID = iss.ID
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("C"))):
			if iss := m.selectedIssue(); iss != nil {
				m.overlay = overlayCommentInput
				m.overlayID = iss.ID
				m.textInput.Reset()
				m.textInput.Focus()
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("Y"))):
			m.statusMsg = "Syncing..."
			result := doSync(m.repos)
			m.statusMsg = result.String()
			m.refreshCurrentView()
			return m, nil
		}

		// View-specific key handling
		switch m.view {
		case viewList:
			return m.updateList(msg)
		case viewKanban:
			return m.updateKanban(msg)
		case viewTree:
			return m.updateTree(msg)
		}
	}

	// Pass through to active sub-component
	var cmd tea.Cmd
	if m.view == viewList {
		if m.focus == focusDetail && m.detail != nil {
			m.viewport, cmd = m.viewport.Update(msg)
		} else {
			m.list, cmd = m.list.Update(msg)
		}
	}
	return m, cmd
}

// selectedIssue returns the currently selected issue across any view.
func (m model) selectedIssue() *issue.Issue {
	switch m.view {
	case viewList:
		if item, ok := m.list.SelectedItem().(issueItem); ok {
			return item.issue
		}
	case viewKanban:
		return m.kanban.selectedIssue()
	case viewTree:
		return m.tree.selectedIssue()
	}
	return nil
}

// refreshCurrentView rebuilds the active view's data after a mutation.
func (m *model) refreshCurrentView() {
	for _, r := range m.repos {
		r.Store.ClearCache()
	}
	switch m.view {
	case viewList:
		m.list = m.buildList()
	case viewKanban:
		m.kanban = newKanbanData(m.repos)
	case viewTree:
		m.tree = newTreeData(m.repos)
	}
	// Refresh detail if open
	if m.detail != nil {
		for _, r := range m.repos {
			if iss, err := r.Store.Get(m.detail.ID); err == nil {
				m.detail = iss
				m.viewport.SetContent(m.renderDetailContent(iss))
				break
			}
		}
	}
}

// updateOverlay handles input when a confirmation or text input overlay is active.
func (m model) updateOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayConfirmClose, overlayConfirmReopen:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("y"))):
			if m.overlay == overlayConfirmClose {
				result := doClose(m.repos, m.overlayID)
				m.statusMsg = result.String()
			} else {
				result := doReopen(m.repos, m.overlayID)
				m.statusMsg = result.String()
			}
			m.overlay = overlayNone
			m.refreshCurrentView()
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("n", "esc"))):
			m.overlay = overlayNone
			m.statusMsg = ""
			return m, nil
		}
		return m, nil

	case overlayCommentInput:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			text := m.textInput.Value()
			if text != "" {
				result := doComment(m.repos, m.overlayID, text)
				m.statusMsg = result.String()
				m.refreshCurrentView()
			}
			m.overlay = overlayNone
			m.textInput.Blur()
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.overlay = overlayNone
			m.textInput.Blur()
			m.statusMsg = ""
			return m, nil
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func switchView(m *model, v viewKind) {
	m.view = v
	m.detail = nil
	m.focus = focusList
	switch v {
	case viewList:
		m.list = m.buildList()
		m.updateLayout()
	case viewKanban:
		m.kanban = newKanbanData(m.repos)
	case viewTree:
		m.tree = newTreeData(m.repos)
	}
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		if m.detail != nil {
			if m.focus == focusList {
				m.focus = focusDetail
			} else {
				m.focus = focusList
			}
		}
		return m, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if m.focus == focusList {
			if item, ok := m.list.SelectedItem().(issueItem); ok {
				if m.detail != nil && m.detail.ID == item.issue.ID {
					m.detail = nil
					m.focus = focusList
				} else {
					m.detail = item.issue
					m.focus = focusDetail
					m.viewport.SetContent(m.renderDetailContent(item.issue))
					m.viewport.GotoTop()
				}
				m.updateLayout()
			}
			return m, nil
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		if m.focus == focusDetail {
			m.focus = focusList
			return m, nil
		}
		if m.detail != nil {
			m.detail = nil
			m.focus = focusList
			m.updateLayout()
			return m, nil
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
		if m.focus == focusList {
			m.statusIdx = (m.statusIdx + 1) % len(statusFilters)
			m.detail = nil
			m.focus = focusList
			m.list = m.buildList()
			m.updateLayout()
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.focus == focusDetail && m.detail != nil {
		m.viewport, cmd = m.viewport.Update(msg)
	} else {
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m model) updateKanban(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("h", "left"))):
		m.kanban.moveLeft()
	case key.Matches(msg, key.NewBinding(key.WithKeys("l", "right"))):
		m.kanban.moveRight()
	case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
		m.kanban.moveDown()
	case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
		m.kanban.moveUp()
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if iss := m.kanban.selectedIssue(); iss != nil {
			if m.detail != nil && m.detail.ID == iss.ID {
				m.detail = nil
			} else {
				m.detail = iss
				m.viewport.SetContent(m.renderDetailContent(iss))
				m.viewport.GotoTop()
			}
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		if m.detail != nil {
			m.detail = nil
		}
	}
	return m, nil
}

func (m model) updateTree(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
		m.tree.moveDown()
	case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
		m.tree.moveUp()
	case key.Matches(msg, key.NewBinding(key.WithKeys("space"))):
		m.tree.toggleExpand()
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if iss := m.tree.selectedIssue(); iss != nil {
			if m.detail != nil && m.detail.ID == iss.ID {
				m.detail = nil
			} else {
				m.detail = iss
				m.viewport.SetContent(m.renderDetailContent(iss))
				m.viewport.GotoTop()
			}
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		if m.detail != nil {
			m.detail = nil
		}
	}
	return m, nil
}

func (m *model) updateLayout() {
	if m.detail != nil {
		m.list.SetSize(m.width/2, m.height)
		m.viewport.SetWidth(m.width - m.width/2 - 3) // 3 for border + padding
		m.viewport.SetHeight(m.height - 2)            // padding
	} else {
		m.list.SetSize(m.width, m.height)
	}
}

func (m model) View() tea.View {
	var content string
	if m.showHelp {
		content = m.helpView()
	} else {
		var mainContent string
		switch m.view {
		case viewList:
			mainContent = m.list.View()
		case viewKanban:
			mainContent = renderKanban(&m.kanban, m.width, m.height)
		case viewTree:
			mainContent = renderTree(&m.tree, m.width, m.height-3)
		}

		if m.detail != nil {
			detailStyle := lipgloss.NewStyle().
				Padding(1, 1).
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true)

			if m.focus == focusList {
				detailStyle = detailStyle.BorderForeground(lipgloss.Color("#555555"))
			} else {
				detailStyle = detailStyle.BorderForeground(lipgloss.Color("#5fafaf"))
			}

			// In kanban mode, detail takes the right half
			detailView := detailStyle.Render(m.viewport.View())
			content = lipgloss.JoinHorizontal(lipgloss.Top, mainContent, detailView)
		} else {
			content = mainContent
		}
	}

	// Overlay
	if m.overlay != overlayNone {
		overlay := m.renderOverlay()
		content = lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}

	// Status message
	if m.statusMsg != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#d7af5f")).
			Padding(0, 1)
		content = lipgloss.JoinVertical(lipgloss.Left, content, statusStyle.Render(m.statusMsg))
	}

	// View tab bar
	tabBar := m.renderTabBar()
	content = lipgloss.JoinVertical(lipgloss.Left, tabBar, content)

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m model) renderOverlay() string {
	style := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color("#ff5f5f"))

	switch m.overlay {
	case overlayConfirmClose:
		return style.Render(fmt.Sprintf("Close %s? (y/n)", m.overlayID))
	case overlayConfirmReopen:
		return style.Render(fmt.Sprintf("Reopen %s? (y/n)", m.overlayID))
	case overlayCommentInput:
		return lipgloss.NewStyle().Padding(0, 2).Render(
			fmt.Sprintf("Comment on %s: %s", m.overlayID, m.textInput.View()),
		)
	}
	return ""
}

func (m model) renderTabBar() string {
	tabs := []struct {
		key   string
		label string
		view  viewKind
	}{
		{"1", "List", viewList},
		{"2", "Kanban", viewKanban},
		{"3", "Tree", viewTree},
	}

	var parts []string
	for _, t := range tabs {
		style := lipgloss.NewStyle().Padding(0, 2)
		label := fmt.Sprintf("[%s] %s", t.key, t.label)
		if t.view == m.view {
			style = style.Bold(true).Foreground(lipgloss.Color("#5fafaf"))
		} else {
			style = style.Foreground(lipgloss.Color("#555555"))
		}
		parts = append(parts, style.Render(label))
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Width(m.width).
		Render(bar)
}

func (m model) renderDetailContent(iss *issue.Issue) string {
	var b strings.Builder

	// Header
	blocked := len(iss.BlockedBy) > 0 && iss.Status != "closed"
	fmt.Fprintf(&b, "%s %s %s", styledStatusIcon(iss.Status, blocked), styledID(iss.ID), styledPriorityBadge(iss.Priority))
	if iss.Type != "" {
		fmt.Fprintf(&b, " [%s]", strings.ToUpper(iss.Type))
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "\n%s\n", lipgloss.NewStyle().Bold(true).Render(iss.Title))

	// Metadata
	if iss.Assignee != "" {
		fmt.Fprintf(&b, "\nAssignee: %s", iss.Assignee)
	}
	if iss.Parent != "" {
		fmt.Fprintf(&b, "\nParent:   %s", styledID(iss.Parent))
	}
	fmt.Fprintf(&b, "\nStatus:   %s", iss.Status)
	if len(iss.Labels) > 0 {
		fmt.Fprintf(&b, "\nLabels:   %s", strings.Join(iss.Labels, ", "))
	}
	if iss.DeferUntil != "" {
		fmt.Fprintf(&b, "\nDeferred: %s", iss.DeferUntil)
	}

	// Description
	if iss.Description != "" {
		fmt.Fprintf(&b, "\n\n%s", iss.Description)
	}

	// Dependencies
	if len(iss.BlockedBy) > 0 {
		fmt.Fprintf(&b, "\n\nBlocked by: %s", strings.Join(iss.BlockedBy, ", "))
	}
	if len(iss.Blocks) > 0 {
		fmt.Fprintf(&b, "\nBlocks:     %s", strings.Join(iss.Blocks, ", "))
	}

	// Comments
	if len(iss.Comments) > 0 {
		fmt.Fprintf(&b, "\n\nComments (%d):", len(iss.Comments))
		for _, c := range iss.Comments {
			author := c.Author
			if author == "" {
				author = "unknown"
			}
			fmt.Fprintf(&b, "\n  [%s] %s: %s", c.Timestamp, author, c.Text)
		}
	}

	return b.String()
}

func (m model) helpView() string {
	help := `Keybindings:

  Navigation
    1/2/3        Switch view: List / Kanban / Tree
    j/k, ↑/↓    Navigate list / scroll detail
    h/l, ←/→    Navigate kanban columns
    space        Toggle expand/collapse (tree view)
    enter        Open detail panel
    tab          Switch focus between list and detail (list view)
    /            Filter issues (fuzzy search, list view)
    s            Cycle status filter (list view)
    esc          Unfocus detail / close detail / clear filter

  Actions
    S            Start issue (open → in_progress)
    X            Close issue (with confirmation)
    O            Reopen issue (with confirmation)
    C            Add comment (text input)
    Y            Sync all repos

  q  Quit    ?  Toggle this help
`
	style := lipgloss.NewStyle().Padding(2, 4)
	return style.Render(help)
}
