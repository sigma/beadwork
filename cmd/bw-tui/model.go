package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jallum/beadwork/internal/issue"
)

// issueItem wraps an issue for the list.Model.
type issueItem struct {
	issue    *issue.Issue
	repoName string // empty when single-repo
}

func (i issueItem) Title() string {
	prefix := ""
	if i.repoName != "" {
		prefix = i.repoName + " "
	}
	return fmt.Sprintf("%s %s%s  %s", statusIcon(i.issue.Status, i.issue), prefix+i.issue.ID, priorityBadge(i.issue.Priority), i.issue.Title)
}

func (i issueItem) Description() string {
	var parts []string
	if i.issue.Type != "" {
		parts = append(parts, strings.ToUpper(i.issue.Type))
	}
	if i.issue.Assignee != "" {
		parts = append(parts, "→ "+i.issue.Assignee)
	}
	if len(i.issue.BlockedBy) > 0 {
		open := 0
		for range i.issue.BlockedBy {
			open++
		}
		parts = append(parts, fmt.Sprintf("blocked by %d", open))
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
	repos    []*RepoSource
	list     list.Model
	detail   *issue.Issue
	width    int
	height   int
	showHelp bool
}

func newModel(repos []*RepoSource) model {
	multiRepo := len(repos) > 1

	var items []list.Item
	for _, r := range repos {
		issues, err := r.Store.List(issue.Filter{})
		if err != nil {
			continue
		}
		for _, iss := range issues {
			name := ""
			if multiRepo {
				name = r.Name
			}
			items = append(items, issueItem{issue: iss, repoName: name})
		}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Beadwork"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	return model{
		repos: repos,
		list:  l,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.detail != nil {
			m.list.SetSize(msg.Width/2, msg.Height)
		} else {
			m.list.SetSize(msg.Width, msg.Height)
		}
		return m, nil

	case tea.KeyMsg:
		// Don't intercept keys while filtering
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q"))):
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if item, ok := m.list.SelectedItem().(issueItem); ok {
				if m.detail != nil && m.detail.ID == item.issue.ID {
					m.detail = nil
					m.list.SetSize(m.width, m.height)
				} else {
					m.detail = item.issue
					m.list.SetSize(m.width/2, m.height)
				}
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			if m.detail != nil {
				m.detail = nil
				m.list.SetSize(m.width, m.height)
				return m, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("?"))):
			m.showHelp = !m.showHelp
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() tea.View {
	var content string
	if m.showHelp {
		content = m.helpView()
	} else if m.detail != nil {
		listView := m.list.View()
		detailView := m.renderDetail(m.detail)
		content = lipgloss.JoinHorizontal(lipgloss.Top, listView, detailView)
	} else {
		content = m.list.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m model) renderDetail(iss *issue.Issue) string {
	w := m.width - m.width/2

	style := lipgloss.NewStyle().
		Width(w).
		Padding(1, 2).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true)

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s\n", statusIcon(iss.Status, iss), iss.ID, priorityBadge(iss.Priority))
	fmt.Fprintf(&b, "\n%s\n", iss.Title)

	if iss.Type != "" {
		fmt.Fprintf(&b, "\nType: %s", strings.ToUpper(iss.Type))
	}
	if iss.Assignee != "" {
		fmt.Fprintf(&b, "\nAssignee: %s", iss.Assignee)
	}
	if iss.Parent != "" {
		fmt.Fprintf(&b, "\nParent: %s", iss.Parent)
	}
	if iss.Status != "" {
		fmt.Fprintf(&b, "\nStatus: %s", iss.Status)
	}
	if len(iss.Labels) > 0 {
		fmt.Fprintf(&b, "\nLabels: %s", strings.Join(iss.Labels, ", "))
	}

	if iss.Description != "" {
		fmt.Fprintf(&b, "\n\n%s", iss.Description)
	}

	if len(iss.BlockedBy) > 0 {
		fmt.Fprintf(&b, "\n\nBlocked by: %s", strings.Join(iss.BlockedBy, ", "))
	}
	if len(iss.Blocks) > 0 {
		fmt.Fprintf(&b, "\nBlocks: %s", strings.Join(iss.Blocks, ", "))
	}

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

	return style.Render(b.String())
}

func (m model) helpView() string {
	help := `Keybindings:

  j/k, ↑/↓    Navigate list
  enter        Toggle detail panel
  /            Filter issues
  esc          Close detail / clear filter
  q            Quit
  ?            Toggle this help
`
	style := lipgloss.NewStyle().Padding(2, 4)
	return style.Render(help)
}

func statusIcon(status string, iss *issue.Issue) string {
	hasOpenBlockers := len(iss.BlockedBy) > 0
	if hasOpenBlockers && status != "closed" {
		return "⊘"
	}
	switch status {
	case "open":
		return "○"
	case "in_progress":
		return "◐"
	case "closed":
		return "✓"
	case "deferred":
		return "❄"
	default:
		return "?"
	}
}

func priorityBadge(p int) string {
	return fmt.Sprintf("P%d", p)
}
