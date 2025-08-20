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
	
	// Render styled footer with proper width
	styledFooter := renderStyledFooter(footer, width)
	
	return content + styledFooter
}

// renderStyledFooter renders the footer with command highlighting and proper width
func renderStyledFooter(text string, width int) string {
	bgColor := lipgloss.Color("#005577")
	fgColor := lipgloss.Color("#FFFFFF")
	cmdColor := lipgloss.Color("#FFFF00")
	
	// First, pad the text to ensure it fills the width (accounting for padding)
	effectiveWidth := width - 2 // -2 for left and right padding
	if len(text) < effectiveWidth {
		text = text + strings.Repeat(" ", effectiveWidth-len(text))
	} else if len(text) > effectiveWidth {
		text = text[:effectiveWidth]
	}
	
	// Now process the text to add highlighting
	var result strings.Builder
	i := 0
	
	for i < len(text) {
		// Find the next word boundary
		wordStart := i
		for i < len(text) && text[i] != ' ' {
			i++
		}
		
		if wordStart < i {
			word := text[wordStart:i]
			
			// Check if word contains a command separator (:)
			if colonIdx := strings.Index(word, ":"); colonIdx > 0 {
				// Highlight the command part
				result.WriteString(lipgloss.NewStyle().
					Foreground(cmdColor).
					Background(bgColor).
					Bold(true).
					Render(word[:colonIdx+1]))
				
				// Normal style for description
				result.WriteString(lipgloss.NewStyle().
					Foreground(fgColor).
					Background(bgColor).
					Render(word[colonIdx+1:]))
			} else {
				// Normal word
				result.WriteString(lipgloss.NewStyle().
					Foreground(fgColor).
					Background(bgColor).
					Render(word))
			}
		}
		
		// Handle spaces
		for i < len(text) && text[i] == ' ' {
			result.WriteString(lipgloss.NewStyle().
				Foreground(fgColor).
				Background(bgColor).
				Render(" "))
			i++
		}
	}
	
	// Wrap with padding and ensure full width
	return lipgloss.NewStyle().
		Background(bgColor).
		Foreground(fgColor).
		Width(width).
		Padding(0, 1).
		Render(result.String())
}