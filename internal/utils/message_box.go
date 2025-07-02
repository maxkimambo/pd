package utils

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// MessageType defines the type of message box to render.
type MessageType int

const (
	// InfoMessage represents an informational message.
	InfoMessage MessageType = iota
	// SuccessMessage represents a success message.
	SuccessMessage
	// WarningMessage represents a warning message.
	WarningMessage
	// ErrorMessage represents an error message.
	ErrorMessage
	// QuestionMessage represents a question or prompt.
	QuestionMessage
)

const (
	infoPrefix     = "ℹ"
	successPrefix  = "✓"
	warningPrefix  = "⚠"
	errorPrefix    = "✗"
	questionPrefix = "?"
)

const (
	topLeft     = "╭"
	topRight    = "╮"
	bottomLeft  = "╰"
	bottomRight = "╯"
	horizontal  = "─"
	vertical    = "│"
)

var (
	infoStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("178"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	questionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
)

// Box is a builder for creating formatted message boxes.
type Box struct {
	messageType MessageType
	title       string
	content     []string
	width       int
}

// NewBox creates a new message box with a specific type.
func NewBox(messageType MessageType, title string) *Box {
	return &Box{
		messageType: messageType,
		title:       title,
		content:     []string{},
		width:       getTerminalWidth() - 8, // Default margin
	}
}

// AddLine adds a line of text to the message box content.
func (b *Box) AddLine(text string) *Box {
	b.content = append(b.content, text)
	return b
}

// AddBullet adds a bulleted line to the message box content.
func (b *Box) AddBullet(text string) *Box {
	b.content = append(b.content, fmt.Sprintf("• %s", text))
	return b
}

// Render builds and returns the formatted message box as a string.
func (b *Box) Render() string {
	style, prefix := b.getStyleAndPrefix()

	allLines := []string{b.title}
	allLines = append(allLines, b.content...)
	message := strings.Join(allLines, "\n")

	return renderStyledBox(message, style, prefix)
}

func (b *Box) getStyleAndPrefix() (lipgloss.Style, string) {
	switch b.messageType {
	case SuccessMessage:
		return successStyle, successPrefix
	case WarningMessage:
		return warningStyle, warningPrefix
	case ErrorMessage:
		return errorStyle, errorPrefix
	case QuestionMessage:
		return questionStyle, questionPrefix
	default:
		return infoStyle, infoPrefix
	}
}

// renderStyledBox handles the actual rendering logic.
func renderStyledBox(message string, style lipgloss.Style, prefix string) string {
	termWidth := getTerminalWidth()
	maxWidth := termWidth - 8
	contentWidth := maxWidth - 6

	var wrappedLines []string
	for _, line := range strings.Split(message, "\n") {
		if utf8.RuneCountInString(line) <= contentWidth {
			wrappedLines = append(wrappedLines, line)
		} else {
			wrappedLines = append(wrappedLines, wrapText(line, contentWidth)...)
		}
	}

	boxWidth := 6
	for _, line := range wrappedLines {
		if lineLen := utf8.RuneCountInString(line); lineLen+6 > boxWidth {
			boxWidth = lineLen + 6
		}
	}

	var sb strings.Builder
	sb.WriteString(style.Render(topLeft + strings.Repeat(horizontal, boxWidth-2) + topRight) + "\n")

	if len(wrappedLines) > 0 {
		firstLine := wrappedLines[0]
		padding := boxWidth - utf8.RuneCountInString(firstLine) - 4 - utf8.RuneCountInString(prefix)
		if padding < 0 {
			padding = 0
		}
		sb.WriteString(fmt.Sprintf("%s %s %s%s %s\n",
			style.Render(vertical),
			style.Bold(true).Render(prefix),
			style.Bold(false).Render(firstLine),
			strings.Repeat(" ", padding),
			style.Render(vertical)))
	}

	for _, line := range wrappedLines[1:] {
		padding := boxWidth - utf8.RuneCountInString(line) - 4
		if padding < 0 {
			padding = 0
		}
		sb.WriteString(fmt.Sprintf("%s   %s%s %s\n",
			style.Render(vertical),
			line,
			strings.Repeat(" ", padding),
			style.Render(vertical)))
	}

	sb.WriteString(style.Render(bottomLeft + strings.Repeat(horizontal, boxWidth-2) + bottomRight))
	return sb.String()
}

// Convenience functions for creating and rendering message boxes.

func Info(title string, lines ...string) string {
	box := NewBox(InfoMessage, title)
	for _, line := range lines {
		box.AddLine(line)
	}
	return box.Render()
}

func Success(title string, lines ...string) string {
	box := NewBox(SuccessMessage, title)
	for _, line := range lines {
		box.AddLine(line)
	}
	return box.Render()
}

func Warning(title string, lines ...string) string {
	box := NewBox(WarningMessage, title)
	for _, line := range lines {
		box.AddLine(line)
	}
	return box.Render()
}

func Error(title string, lines ...string) string {
	box := NewBox(ErrorMessage, title)
	for _, line := range lines {
		box.AddLine(line)
	}
	return box.Render()
}

func Question(title string, lines ...string) string {
	box := NewBox(QuestionMessage, title)
	for _, line := range lines {
		box.AddLine(line)
	}
	return box.Render()
}

// getTerminalWidth returns the terminal width or defaults to 80 if unable to detect.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80
	}
	return width
}

// wrapText wraps text to fit within the specified maximum width.
func wrapText(text string, maxWidth int) []string {
	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	currentLine := words[0]
	currentWidth := utf8.RuneCountInString(currentLine)

	for _, word := range words[1:] {
		wordWidth := utf8.RuneCountInString(word)
		
		if currentWidth+wordWidth+1 <= maxWidth {
			currentLine += " " + word
			currentWidth += wordWidth + 1
		} else {
			lines = append(lines, currentLine)
			currentLine = word
			currentWidth = wordWidth
		}
	}
	
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	
	return lines
}
