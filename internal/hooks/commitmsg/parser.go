package commitmsg

import (
	"strings"
)

// ParsedCommitMessage represents a commit message split into sections.
type ParsedCommitMessage struct {
	Raw    string
	Title  string
	Body   string
	Footer string
}

// ParseCommitMessage parses a commit message into title, body, and footer.
//
// Parsing rules:
// - Sections are separated by empty lines (lines with only whitespace)
// - Title: First section (always present)
// - Footer: Last section (after final empty line), if 2+ sections exist
// - Body: All middle sections (between title and footer), if 3+ sections exist.
func ParseCommitMessage(message string) ParsedCommitMessage {
	// Normalize line endings
	message = strings.ReplaceAll(message, "\r\n", "\n")
	message = strings.TrimRight(message, "\n")

	// Split into sections by empty lines
	sections := splitIntoSections(message)

	result := ParsedCommitMessage{
		Raw:    message,
		Title:  "",
		Body:   "",
		Footer: "",
	}

	if len(sections) == 0 {
		return result
	}

	// Title is always the first section
	result.Title = sections[0]

	const twoSections = 2
	if len(sections) == 1 {
		// Only title, no body or footer
		return result
	}

	if len(sections) == twoSections {
		// Title + Footer (no body)
		result.Footer = sections[1]
		return result
	}

	// 3+ sections: Title + Body + Footer
	result.Footer = sections[len(sections)-1]

	// Body is everything between title and footer
	bodyParts := sections[1 : len(sections)-1]
	result.Body = strings.Join(bodyParts, "\n\n")

	return result
}

// splitIntoSections splits a message by empty lines into sections.
func splitIntoSections(message string) []string {
	lines := strings.Split(message, "\n")

	var sections []string
	currentSection := make([]string, 0, len(lines))

	for _, line := range lines {
		if isEmptyLine(line) {
			// Empty line marks section boundary
			if len(currentSection) > 0 {
				sections = append(sections, strings.Join(currentSection, "\n"))
				currentSection = nil
			}

			continue
		}

		currentSection = append(currentSection, line)
	}

	// Add final section
	if len(currentSection) > 0 {
		sections = append(sections, strings.Join(currentSection, "\n"))
	}

	return sections
}

func isEmptyLine(line string) bool {
	return strings.TrimSpace(line) == ""
}
