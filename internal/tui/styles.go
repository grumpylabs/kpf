package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Admin header style - blue background matching admin exactly
	adminHeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#005577")).
		Bold(true).
		Padding(0, 1)

	// Section header style - yellow text matching admin exactly
	sectionHeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFF00")).
		Bold(false)

	// Table header style - blue background matching admin exactly
	adminTableHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#005577")).
		Padding(0, 1)

	// Normal row style - light text on dark background
	adminNormalRowStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E0E0E0"))

	// Selected row style - black background with yellow foreground
	adminSelectedRowStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFF00")).
		Background(lipgloss.Color("#000000")).
		Bold(true)

	// Status indicators
	activeStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00FF00"))

	inactiveStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	// Detail view styles
	detailBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#005577")).
		Padding(1, 2).
		Background(lipgloss.Color("#000000")).
		Foreground(lipgloss.Color("#FFFFFF"))

	detailLabelStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00AAFF"))

	// Empty state
	emptyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Italic(true).
		MarginTop(2)

	// Error and success
	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000")).
		Bold(true)

	successStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF00")).
		Bold(true)
)

// RenderWithFooter renders content with a footer positioned at the bottom of the terminal
// This matches the admin UI footer implementation exactly
func RenderWithFooter(content, footer string, width, height int) string {
	// Calculate how many lines the content takes up
	contentLines := strings.Split(content, "\n")
	contentHeight := len(contentLines)
	
	// Calculate available space for padding
	footerHeight := 1 // Footer is one line
	availableHeight := height - footerHeight
	
	// If content fits, pad with empty lines to push footer to bottom
	if contentHeight < availableHeight {
		padding := availableHeight - contentHeight
		for i := 0; i < padding; i++ {
			content += "\n"
		}
	} else if contentHeight > availableHeight {
		// If content is too tall, truncate it to fit
		lines := strings.Split(content, "\n")
		if len(lines) > availableHeight {
			lines = lines[:availableHeight]
		}
		content = strings.Join(lines, "\n")
	}
	
	// Style the footer to match admin colors with command highlighting
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#005577")).
		Width(width).
		Padding(0, 1)
	
	// Highlight command characters matching admin style exactly
	styledFooter := highlightCommands(footer)
	
	return content + footerStyle.Render(styledFooter)
}

// highlightCommands highlights commands in footer text matching admin style exactly
func highlightCommands(text string) string {
	// Split by spaces to process each command individually
	parts := strings.Fields(text)
	var result []string
	
	// Define consistent background for all parts
	bgColor := lipgloss.Color("#005577")
	
	for _, part := range parts {
		// Look for patterns like "↑↓/jk:navigate" or "Enter:view" etc.
		if strings.Contains(part, ":") {
			colonIndex := strings.Index(part, ":")
			if colonIndex > 0 {
				cmdPart := part[:colonIndex+1] // Include the colon
				descPart := part[colonIndex+1:] // Everything after colon
				
				// Highlight the command part with yellow text matching admin
				highlightedCmd := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFF00")).
					Background(bgColor).
					Bold(true).
					Render(cmdPart)
				
				// Regular part with consistent background
				regularPart := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFFFF")).
					Background(bgColor).
					Render(descPart)
				
				result = append(result, highlightedCmd+regularPart)
			} else {
				// No command part, render with normal style
				styledPart := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFFFF")).
					Background(bgColor).
					Render(part)
				result = append(result, styledPart)
			}
		} else {
			// No colon, render with normal style
			styledPart := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(bgColor).
				Render(part)
			result = append(result, styledPart)
		}
	}
	
	// Join with styled spaces to maintain background
	spacer := lipgloss.NewStyle().
		Background(bgColor).
		Render(" ")
	
	return strings.Join(result, spacer)
}