package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jallum/beadwork/internal/issue"
)

// Status filter options, cycled with 's'.
var statusFilters = []string{"", "open", "in_progress", "closed", "deferred"}

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
	list      list.Model
	detail    *issue.Issue
	viewport  viewport.Model
	focus     focus
	width     int
	height    int
	showHelp  bool
	statusIdx int // index into statusFilters
}

func newModel(repos []*RepoSource) model {
	m := model{
		repos:    repos,
		viewport: viewport.New(),
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
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tea.KeyMsg:
		// Don't intercept keys while filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q"))):
			return m, tea.Quit
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
		case key.Matches(msg, key.NewBinding(key.WithKeys("?"))):
			m.showHelp = !m.showHelp
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
	} else if m.detail != nil {
		listStyle := lipgloss.NewStyle()
		detailStyle := lipgloss.NewStyle().
			Padding(1, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderLeft(true)

		if m.focus == focusList {
			detailStyle = detailStyle.BorderForeground(lipgloss.Color("#555555"))
		} else {
			detailStyle = detailStyle.BorderForeground(lipgloss.Color("#5fafaf"))
		}

		listView := listStyle.Render(m.list.View())
		detailView := detailStyle.Render(m.viewport.View())
		content = lipgloss.JoinHorizontal(lipgloss.Top, listView, detailView)
	} else {
		content = m.list.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
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

  j/k, ↑/↓    Navigate list / scroll detail
  enter        Open detail panel
  tab          Switch focus between list and detail
  /            Filter issues (fuzzy search)
  s            Cycle status filter (all → open → in_progress → closed → deferred)
  esc          Unfocus detail / close detail / clear filter
  q            Quit
  ?            Toggle this help
`
	style := lipgloss.NewStyle().Padding(2, 4)
	return style.Render(help)
}
