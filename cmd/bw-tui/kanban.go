package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jallum/beadwork/internal/issue"
)

// kanbanColumns defines the columns and their display order.
var kanbanColumns = []struct {
	status string
	title  string
}{
	{"open", "Open"},
	{"in_progress", "In Progress"},
	{"closed", "Closed"},
	{"deferred", "Deferred"},
}

// kanbanData holds the categorized issues for the kanban view.
type kanbanData struct {
	columns   [4][]issueItem // indexed by kanbanColumns order
	colIdx    int            // currently focused column
	rowIdx    int            // currently focused row within column
	multiRepo bool
}

func newKanbanData(repos []*RepoSource) kanbanData {
	multiRepo := len(repos) > 1
	kd := kanbanData{multiRepo: multiRepo}

	for _, r := range repos {
		for colIdx, col := range kanbanColumns {
			issues, err := r.Store.List(issue.Filter{Status: col.status})
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
				kd.columns[colIdx] = append(kd.columns[colIdx], issueItem{
					issue:        iss,
					repoName:     name,
					openBlockers: openBlockers,
				})
			}
		}
	}

	return kd
}

func (kd *kanbanData) moveLeft() {
	if kd.colIdx > 0 {
		kd.colIdx--
		kd.clampRow()
	}
}

func (kd *kanbanData) moveRight() {
	if kd.colIdx < len(kanbanColumns)-1 {
		kd.colIdx++
		kd.clampRow()
	}
}

func (kd *kanbanData) moveUp() {
	if kd.rowIdx > 0 {
		kd.rowIdx--
	}
}

func (kd *kanbanData) moveDown() {
	col := kd.columns[kd.colIdx]
	if kd.rowIdx < len(col)-1 {
		kd.rowIdx++
	}
}

func (kd *kanbanData) clampRow() {
	col := kd.columns[kd.colIdx]
	if kd.rowIdx >= len(col) {
		if len(col) > 0 {
			kd.rowIdx = len(col) - 1
		} else {
			kd.rowIdx = 0
		}
	}
}

func (kd *kanbanData) selectedIssue() *issue.Issue {
	col := kd.columns[kd.colIdx]
	if kd.rowIdx < len(col) {
		return col[kd.rowIdx].issue
	}
	return nil
}

func renderKanban(kd *kanbanData, width, height int) string {
	colWidth := width / len(kanbanColumns)
	if colWidth < 20 {
		colWidth = 20
	}
	contentWidth := colWidth - 4 // padding + border

	var rendered []string
	for colIdx, col := range kanbanColumns {
		items := kd.columns[colIdx]
		isFocused := colIdx == kd.colIdx

		// Column header
		headerStyle := lipgloss.NewStyle().
			Bold(true).
			Width(contentWidth).
			Align(lipgloss.Center).
			Padding(0, 1)

		header := fmt.Sprintf("%s (%d)", col.title, len(items))
		if isFocused {
			headerStyle = headerStyle.Foreground(lipgloss.Color("#5fafaf"))
		}

		var b strings.Builder
		b.WriteString(headerStyle.Render(header))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", contentWidth))
		b.WriteString("\n")

		// Cards
		maxCards := height - 5 // header + separator + padding
		if maxCards < 1 {
			maxCards = 1
		}
		for i, item := range items {
			if i >= maxCards {
				remaining := len(items) - maxCards
				b.WriteString(fmt.Sprintf("\n  +%d more", remaining))
				break
			}

			isSelected := isFocused && i == kd.rowIdx
			card := renderKanbanCard(item, contentWidth, isSelected)
			b.WriteString(card)
			b.WriteString("\n")
		}

		if len(items) == 0 {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("#555555")).
				Italic(true).
				Render("  (empty)"))
		}

		colHeight := height - 4 // account for tab bar and borders
		if colHeight < 3 {
			colHeight = 3
		}

		colStyle := lipgloss.NewStyle().
			Width(colWidth).
			Height(colHeight).
			Padding(1, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(colIdx < len(kanbanColumns)-1)

		rendered = append(rendered, colStyle.Render(b.String()))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func renderKanbanCard(item issueItem, width int, selected bool) string {
	iss := item.issue
	blocked := item.openBlockers > 0 && iss.Status != "closed"

	icon := styledStatusIcon(iss.Status, blocked)
	badge := styledPriorityBadge(iss.Priority)
	id := styledID(iss.ID)

	// Truncate title to fit
	titleMax := width - 12 // icon + space + id + space + badge + padding
	title := iss.Title
	if len(title) > titleMax && titleMax > 3 {
		title = title[:titleMax-1] + "…"
	}

	line := fmt.Sprintf("%s %s %s %s", icon, id, badge, title)

	style := lipgloss.NewStyle().Width(width).Padding(0, 1)
	if selected {
		style = style.
			Background(lipgloss.Color("#333333")).
			Foreground(lipgloss.Color("#ffffff"))
	}

	return style.Render(line)
}
