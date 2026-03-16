package main

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Colors matching beadwork's existing palette.
var (
	colorP0 = lipgloss.Color("#ff5f5f") // bright red
	colorP1 = lipgloss.Color("#d75f5f") // red
	colorP2 = lipgloss.Color("#d7af5f") // yellow
	colorP3 = lipgloss.Color("#5fafaf") // cyan
	colorP4 = lipgloss.Color("#808080") // dim

	colorOpen       = lipgloss.Color("#808080")
	colorInProgress = lipgloss.Color("#d7af5f")
	colorClosed     = lipgloss.Color("#5faf5f")
	colorBlocked    = lipgloss.Color("#d75f5f")
	colorDeferred   = lipgloss.Color("#5f87af")
	colorID         = lipgloss.Color("#5fafaf")
	colorRepo       = lipgloss.Color("#af87d7")

	styleID = lipgloss.NewStyle().Foreground(colorID)
)

var priorityColors = [5]color.Color{colorP0, colorP1, colorP2, colorP3, colorP4}

func styledPriorityBadge(p int) string {
	if p < 0 || p > 4 {
		p = 2
	}
	s := lipgloss.NewStyle().Foreground(priorityColors[p])
	return s.Render(fmt.Sprintf("P%d", p))
}

func styledStatusIcon(status string, blocked bool) string {
	if blocked {
		return lipgloss.NewStyle().Foreground(colorBlocked).Render("⊘")
	}
	switch status {
	case "open":
		return lipgloss.NewStyle().Foreground(colorOpen).Render("○")
	case "in_progress":
		return lipgloss.NewStyle().Foreground(colorInProgress).Render("◐")
	case "closed":
		return lipgloss.NewStyle().Foreground(colorClosed).Render("✓")
	case "deferred":
		return lipgloss.NewStyle().Foreground(colorDeferred).Render("❄")
	default:
		return "?"
	}
}

func styledID(id string) string {
	return styleID.Render(id)
}

func styledRepoName(name string) string {
	return lipgloss.NewStyle().Foreground(colorRepo).Render(name)
}
