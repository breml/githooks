package commitmsg_test

import (
	"strings"
	"testing"

	"github.com/breml/githooks/internal/hooks/commitmsg"
)

func TestParseCommitMessage(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		wantTitle  string
		wantBody   string
		wantFooter string
	}{
		{
			name:       "title only",
			message:    "Add feature",
			wantTitle:  "Add feature",
			wantBody:   "",
			wantFooter: "",
		},
		{
			name:       "title only with trailing newline",
			message:    "Add feature\n",
			wantTitle:  "Add feature",
			wantBody:   "",
			wantFooter: "",
		},
		{
			name:       "title and footer",
			message:    "Add feature\n\nSigned-off-by: John <j@ex.com>",
			wantTitle:  "Add feature",
			wantBody:   "",
			wantFooter: "Signed-off-by: John <j@ex.com>",
		},
		{
			name:       "title and footer with trailing newlines",
			message:    "Add feature\n\nSigned-off-by: John <j@ex.com>\n\n",
			wantTitle:  "Add feature",
			wantBody:   "",
			wantFooter: "Signed-off-by: John <j@ex.com>",
		},
		{
			name:       "title, body, and footer",
			message:    "Add feature\n\nThis adds X.\n\nFixes #123",
			wantTitle:  "Add feature",
			wantBody:   "This adds X.",
			wantFooter: "Fixes #123",
		},
		{
			name:       "title, multi-line body, and footer",
			message:    "Add feature\n\nThis adds X.\nIt also adds Y.\n\nFixes #123",
			wantTitle:  "Add feature",
			wantBody:   "This adds X.\nIt also adds Y.",
			wantFooter: "Fixes #123",
		},
		{
			name:       "multiple body sections",
			message:    "Add feature\n\nFirst para.\n\nSecond para.\n\nFixes #123",
			wantTitle:  "Add feature",
			wantBody:   "First para.\n\nSecond para.",
			wantFooter: "Fixes #123",
		},
		{
			name:       "multiple body sections with multi-line paragraphs",
			message:    "Add feature\n\nFirst para line 1.\nFirst para line 2.\n\nSecond para line 1.\nSecond para line 2.\n\nFixes #123\nSigned-off-by: John <j@ex.com>",
			wantTitle:  "Add feature",
			wantBody:   "First para line 1.\nFirst para line 2.\n\nSecond para line 1.\nSecond para line 2.",
			wantFooter: "Fixes #123\nSigned-off-by: John <j@ex.com>",
		},
		{
			name:       "empty message",
			message:    "",
			wantTitle:  "",
			wantBody:   "",
			wantFooter: "",
		},
		{
			name:       "only empty lines",
			message:    "\n\n\n",
			wantTitle:  "",
			wantBody:   "",
			wantFooter: "",
		},
		{
			name:       "title with multiple empty lines before footer",
			message:    "Add feature\n\n\n\nSigned-off-by: John <j@ex.com>",
			wantTitle:  "Add feature",
			wantBody:   "",
			wantFooter: "Signed-off-by: John <j@ex.com>",
		},
		{
			name:       "Windows line endings (CRLF)",
			message:    "Add feature\r\n\r\nThis is body.\r\n\r\nFixes #123",
			wantTitle:  "Add feature",
			wantBody:   "This is body.",
			wantFooter: "Fixes #123",
		},
		{
			name:       "mixed line endings",
			message:    "Add feature\r\n\nThis is body.\n\r\nFixes #123",
			wantTitle:  "Add feature",
			wantBody:   "This is body.",
			wantFooter: "Fixes #123",
		},
		{
			name:       "title with spaces (should be preserved)",
			message:    "  Add feature with spaces  \n\nBody text\n\nFooter",
			wantTitle:  "  Add feature with spaces  ",
			wantBody:   "Body text",
			wantFooter: "Footer",
		},
		{
			name:       "complex real-world commit message",
			message:    "feat(api): Add user authentication endpoint\n\nThis commit introduces a new REST API endpoint for user authentication.\nIt supports JWT token generation and validation.\n\nThe implementation includes:\n- Password hashing with bcrypt\n- Token expiration handling\n- Refresh token support\n\nBreaking Change: The /login endpoint now requires a different payload format.\n\nFixes #456\nCloses #789\nSigned-off-by: Jane Doe <jane@example.com>",
			wantTitle:  "feat(api): Add user authentication endpoint",
			wantBody:   "This commit introduces a new REST API endpoint for user authentication.\nIt supports JWT token generation and validation.\n\nThe implementation includes:\n- Password hashing with bcrypt\n- Token expiration handling\n- Refresh token support\n\nBreaking Change: The /login endpoint now requires a different payload format.",
			wantFooter: "Fixes #456\nCloses #789\nSigned-off-by: Jane Doe <jane@example.com>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := commitmsg.ParseCommitMessage(tt.message)

			if parsed.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", parsed.Title, tt.wantTitle)
			}

			if parsed.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", parsed.Body, tt.wantBody)
			}

			if parsed.Footer != tt.wantFooter {
				t.Errorf("Footer = %q, want %q", parsed.Footer, tt.wantFooter)
			}

			// Raw should be the normalized version (without trailing newlines, with \n)
			if parsed.Raw != tt.message &&
				parsed.Raw != strings.TrimRight(strings.ReplaceAll(tt.message, "\r\n", "\n"), "\n") {
				// Allow for normalization
				normalized := strings.TrimRight(strings.ReplaceAll(tt.message, "\r\n", "\n"), "\n")
				if parsed.Raw != normalized {
					t.Errorf("Raw = %q, want %q (normalized from %q)", parsed.Raw, normalized, tt.message)
				}
			}
		})
	}
}
