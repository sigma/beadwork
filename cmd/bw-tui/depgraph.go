package main

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jallum/beadwork/internal/issue"
)

// depGraphData holds the dependency graph state.
type depGraphData struct {
	nodes  []depNode   // all issues that participate in dependencies
	flat   []int       // indices into nodes for cursor navigation
	cursor int
}

type depNode struct {
	issue        *issue.Issue
	repoName     string
	openBlockers int
	blocks       []string // IDs this issue blocks
	blockedBy    []string // IDs that block this issue
	expanded     bool
}

func newDepGraphData(repos []*RepoSource, filterText string, statusFilter string) depGraphData {
	multiRepo := len(repos) > 1
	dg := depGraphData{}

	// Collect all issues that have any dependency relationships
	seen := make(map[string]bool)
	var nodes []depNode

	for _, r := range repos {
		allIssues, err := r.Store.List(issue.Filter{})
		if err != nil {
			continue
		}

		for _, iss := range allIssues {
			if len(iss.Blocks) == 0 && len(iss.BlockedBy) == 0 {
				continue
			}
			if seen[iss.ID] {
				continue
			}
			seen[iss.ID] = true

			if statusFilter != "" && iss.Status != statusFilter {
				continue
			}

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
			item := issueItem{issue: iss, repoName: name, openBlockers: openBlockers}
			if !item.matchesFilter(filterText) {
				continue
			}
			nodes = append(nodes, depNode{
				issue:        iss,
				repoName:     name,
				openBlockers: openBlockers,
				blocks:       iss.Blocks,
				blockedBy:    iss.BlockedBy,
				expanded:     true,
			})
		}
	}

	// Sort: roots first (nothing blocks them), then by priority
	sort.Slice(nodes, func(i, j int) bool {
		iIsRoot := len(nodes[i].blockedBy) == 0
		jIsRoot := len(nodes[j].blockedBy) == 0
		if iIsRoot != jIsRoot {
			return iIsRoot
		}
		if nodes[i].issue.Priority != nodes[j].issue.Priority {
			return nodes[i].issue.Priority < nodes[j].issue.Priority
		}
		return nodes[i].issue.ID < nodes[j].issue.ID
	})

	dg.nodes = nodes

	// Build ID → index lookup for edge rendering and auto-collapse
	idIdx := make(map[string]int)
	for i, n := range dg.nodes {
		idIdx[n.issue.ID] = i
	}

	// Auto-collapse nodes whose blocked issues are all closed
	for i := range dg.nodes {
		if len(dg.nodes[i].blocks) > 0 {
			allClosed := true
			for _, blockedID := range dg.nodes[i].blocks {
				if idx, exists := idIdx[blockedID]; exists {
					if dg.nodes[idx].issue.Status != "closed" {
						allClosed = false
						break
					}
				} else {
					// Target not in node list (filtered out) — keep expanded
					allClosed = false
					break
				}
			}
			if allClosed {
				dg.nodes[i].expanded = false
			}
		}
	}

	// Build set of IDs that appear as edge targets (blocked by another node).
	// These should not appear as top-level rows since they're already shown
	// as edges under their blockers.
	isTarget := make(map[string]bool)
	for _, n := range nodes {
		for _, blockedID := range n.blocks {
			isTarget[blockedID] = true
		}
	}

	for i, n := range nodes {
		if !isTarget[n.issue.ID] {
			dg.flat = append(dg.flat, i)
		}
	}

	return dg
}

func (dg *depGraphData) moveUp() {
	if dg.cursor > 0 {
		dg.cursor--
	}
}

func (dg *depGraphData) moveDown() {
	if dg.cursor < len(dg.flat)-1 {
		dg.cursor++
	}
}

func (dg *depGraphData) pageUp(n int) {
	if n < 1 {
		n = 1
	}
	dg.cursor -= n
	if dg.cursor < 0 {
		dg.cursor = 0
	}
}

func (dg *depGraphData) pageDown(n int) {
	if n < 1 {
		n = 1
	}
	dg.cursor += n
	if dg.cursor >= len(dg.flat) {
		if len(dg.flat) > 0 {
			dg.cursor = len(dg.flat) - 1
		} else {
			dg.cursor = 0
		}
	}
}

func (dg *depGraphData) toggleExpand() {
	if dg.cursor < len(dg.flat) {
		idx := dg.flat[dg.cursor]
		if len(dg.nodes[idx].blocks) > 0 {
			dg.nodes[idx].expanded = !dg.nodes[idx].expanded
		}
	}
}

func (dg *depGraphData) expand() {
	if dg.cursor < len(dg.flat) {
		idx := dg.flat[dg.cursor]
		if len(dg.nodes[idx].blocks) > 0 {
			dg.nodes[idx].expanded = true
		}
	}
}

func (dg *depGraphData) collapse() {
	if dg.cursor < len(dg.flat) {
		idx := dg.flat[dg.cursor]
		if len(dg.nodes[idx].blocks) > 0 {
			dg.nodes[idx].expanded = false
		}
	}
}

func (dg *depGraphData) selectedIssue() *issue.Issue {
	if dg.cursor < len(dg.flat) {
		return dg.nodes[dg.flat[dg.cursor]].issue
	}
	return nil
}

func renderDepGraph(dg *depGraphData, width, height int, statusFilter string) string {
	if len(dg.nodes) == 0 {
		return lipgloss.NewStyle().Padding(2, 4).
			Foreground(lipgloss.Color("#555555")).
			Render("No dependency relationships found.")
	}

	var b strings.Builder
	title := "Dependency Graph"
	if statusFilter != "" {
		title += " [" + statusFilter + "]"
	} else {
		title += " [all]"
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(title))
	b.WriteString("\n\n")

	// Build ID → node index for edge rendering
	idIdx := make(map[string]int)
	for i, n := range dg.nodes {
		idIdx[n.issue.ID] = i
	}

	visibleHeight := height - 6
	if visibleHeight < 1 {
		visibleHeight = 1
	}
	startIdx := 0
	if dg.cursor >= visibleHeight {
		startIdx = dg.cursor - visibleHeight + 1
	}
	endIdx := startIdx + visibleHeight
	if endIdx > len(dg.flat) {
		endIdx = len(dg.flat)
	}

	for i := startIdx; i < endIdx; i++ {
		nodeIdx := dg.flat[i]
		node := dg.nodes[nodeIdx]
		selected := i == dg.cursor

		line := renderDepLine(node, selected)
		b.WriteString(line)
		b.WriteString("\n")

		// Draw edges to blocked issues (only when expanded)
		if node.expanded && len(node.blocks) > 0 {
			for j, blockedID := range node.blocks {
				isLast := j == len(node.blocks)-1
				connector := "├─▶"
				if isLast {
					connector = "└─▶"
				}
				edgeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

				// Show full issue info on the edge line
				targetLabel := styledID(blockedID)
				if idx, exists := idIdx[blockedID]; exists {
					target := dg.nodes[idx]
					blocked := target.openBlockers > 0 && target.issue.Status != "closed"
					icon := styledStatusIcon(target.issue.Status, blocked)
					badge := styledPriorityBadge(target.issue.Priority)
					targetLabel = fmt.Sprintf("%s %s %s %s", icon, styledID(blockedID), badge, target.issue.Title)
				}

				b.WriteString(edgeStyle.Render("    " + connector + " "))
				b.WriteString(targetLabel)
				b.WriteString("\n")
			}
		}
	}

	return lipgloss.NewStyle().Padding(1, 2).Width(width).Render(b.String())
}

func renderDepLine(node depNode, selected bool) string {
	iss := node.issue
	blocked := node.openBlockers > 0 && iss.Status != "closed"

	expandIcon := " "
	if len(node.blocks) > 0 {
		if node.expanded {
			expandIcon = "▼"
		} else {
			expandIcon = "▶"
		}
	}

	icon := styledStatusIcon(iss.Status, blocked)
	id := styledID(iss.ID)
	badge := styledPriorityBadge(iss.Priority)

	// Show blocker count
	depInfo := ""
	if len(node.blockedBy) > 0 {
		openCount := node.openBlockers
		if openCount > 0 {
			depInfo = fmt.Sprintf(" [blocked by %d]", openCount)
		}
	}

	line := fmt.Sprintf("%s %s %s %s %s%s", expandIcon, icon, id, badge, iss.Title, depInfo)

	if selected {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("#333333")).
			Foreground(lipgloss.Color("#ffffff")).
			Render(line)
	}

	return line
}
