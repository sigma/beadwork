package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/jallum/beadwork/internal/issue"
)

// treeNode represents an issue in the tree hierarchy.
type treeNode struct {
	item     issueItem
	children []*treeNode
	expanded bool
}

// treeData holds the tree view state.
type treeData struct {
	roots    []*treeNode       // top-level issues (no parent, or parent is epic)
	flat     []*treeNode       // flattened visible list for cursor navigation
	cursor   int               // index into flat
}

func newTreeData(repos []*RepoSource) treeData {
	multiRepo := len(repos) > 1
	td := treeData{}

	for _, r := range repos {
		// Load all issues
		allIssues, err := r.Store.List(issue.Filter{})
		if err != nil {
			continue
		}

		// Build lookup maps
		byID := make(map[string]*issue.Issue)
		childrenOf := make(map[string][]*issue.Issue)
		for _, iss := range allIssues {
			byID[iss.ID] = iss
			if iss.Parent != "" {
				childrenOf[iss.Parent] = append(childrenOf[iss.Parent], iss)
			}
		}

		// Build tree nodes for root issues (no parent or parent not in set)
		for _, iss := range allIssues {
			if iss.Parent != "" {
				continue // will be added as child
			}
			name := ""
			if multiRepo {
				name = r.Name
			}
			node := buildTreeNode(iss, childrenOf, r.Store, name)
			td.roots = append(td.roots, node)
		}
	}

	td.rebuildFlat()
	return td
}

func buildTreeNode(iss *issue.Issue, childrenOf map[string][]*issue.Issue, store *issue.Store, repoName string) *treeNode {
	openBlockers := 0
	for _, bid := range iss.BlockedBy {
		if !store.IsClosed(bid) {
			openBlockers++
		}
	}

	node := &treeNode{
		item: issueItem{
			issue:        iss,
			repoName:     repoName,
			openBlockers: openBlockers,
		},
		expanded: true, // expanded by default
	}

	for _, child := range childrenOf[iss.ID] {
		childNode := buildTreeNode(child, childrenOf, store, repoName)
		node.children = append(node.children, childNode)
	}

	return node
}

// rebuildFlat constructs the flat visible list from the tree.
func (td *treeData) rebuildFlat() {
	td.flat = td.flat[:0]
	for _, root := range td.roots {
		td.flattenNode(root, 0)
	}
	if td.cursor >= len(td.flat) {
		if len(td.flat) > 0 {
			td.cursor = len(td.flat) - 1
		} else {
			td.cursor = 0
		}
	}
}

func (td *treeData) flattenNode(node *treeNode, depth int) {
	td.flat = append(td.flat, node)
	if node.expanded {
		for _, child := range node.children {
			td.flattenNode(child, depth+1)
		}
	}
}

func (td *treeData) moveUp() {
	if td.cursor > 0 {
		td.cursor--
	}
}

func (td *treeData) moveDown() {
	if td.cursor < len(td.flat)-1 {
		td.cursor++
	}
}

func (td *treeData) toggleExpand() {
	if td.cursor < len(td.flat) {
		node := td.flat[td.cursor]
		if len(node.children) > 0 {
			node.expanded = !node.expanded
			td.rebuildFlat()
		}
	}
}

func (td *treeData) selectedIssue() *issue.Issue {
	if td.cursor < len(td.flat) {
		return td.flat[td.cursor].item.issue
	}
	return nil
}

// nodeDepth returns the depth of a node by walking the flat list.
func (td *treeData) nodeDepth(idx int) int {
	if idx >= len(td.flat) {
		return 0
	}
	target := td.flat[idx]

	// Walk roots to find depth
	for _, root := range td.roots {
		if d := findDepth(root, target, 0); d >= 0 {
			return d
		}
	}
	return 0
}

func findDepth(node *treeNode, target *treeNode, depth int) int {
	if node == target {
		return depth
	}
	for _, child := range node.children {
		if d := findDepth(child, target, depth+1); d >= 0 {
			return d
		}
	}
	return -1
}

func renderTree(td *treeData, width, height int) string {
	if len(td.flat) == 0 {
		return lipgloss.NewStyle().Padding(2, 4).
			Foreground(lipgloss.Color("#555555")).
			Render("No issues with parent-child relationships found.")
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Issue Tree"))
	b.WriteString("\n\n")

	// Determine visible window
	visibleHeight := height - 6 // header + padding
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	startIdx := 0
	if td.cursor >= visibleHeight {
		startIdx = td.cursor - visibleHeight + 1
	}
	endIdx := startIdx + visibleHeight
	if endIdx > len(td.flat) {
		endIdx = len(td.flat)
	}

	for i := startIdx; i < endIdx; i++ {
		node := td.flat[i]
		depth := td.nodeDepth(i)
		selected := i == td.cursor

		line := renderTreeLine(node, depth, selected)
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Progress indicator
	b.WriteString(fmt.Sprintf("\n  %d/%d issues", td.cursor+1, len(td.flat)))

	return lipgloss.NewStyle().Padding(1, 2).Width(width).Render(b.String())
}

func renderTreeLine(node *treeNode, depth int, selected bool) string {
	iss := node.item.issue
	blocked := node.item.openBlockers > 0 && iss.Status != "closed"

	indent := strings.Repeat("  ", depth)

	// Tree connector
	expandIcon := " "
	if len(node.children) > 0 {
		if node.expanded {
			expandIcon = "▼"
		} else {
			expandIcon = "▶"
		}
	}

	icon := styledStatusIcon(iss.Status, blocked)
	id := styledID(iss.ID)
	badge := styledPriorityBadge(iss.Priority)

	// Progress for parent nodes
	progress := ""
	if len(node.children) > 0 {
		closed := 0
		for _, ch := range node.children {
			if ch.item.issue.Status == "closed" {
				closed++
			}
		}
		progress = fmt.Sprintf(" (%d/%d)", closed, len(node.children))
	}

	line := fmt.Sprintf("%s%s %s %s %s %s%s", indent, expandIcon, icon, id, badge, iss.Title, progress)

	if selected {
		return lipgloss.NewStyle().
			Background(lipgloss.Color("#333333")).
			Foreground(lipgloss.Color("#ffffff")).
			Render(line)
	}

	return line
}
