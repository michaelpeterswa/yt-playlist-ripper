package ytdl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSummarizeIssues(t *testing.T) {
	t.Run("empty input returns empty string", func(t *testing.T) {
		assert.Equal(t, "", summarizeIssues(nil))
		assert.Equal(t, "", summarizeIssues([]string{}))
	})

	t.Run("single line passes through unchanged", func(t *testing.T) {
		assert.Equal(t, "ERROR: boom", summarizeIssues([]string{"ERROR: boom"}))
	})

	t.Run("duplicates collapse with count in first-occurrence order", func(t *testing.T) {
		got := summarizeIssues([]string{
			"WARNING: [youtube] A: missing url",
			"ERROR: 403",
			"WARNING: [youtube] B: missing url",
			"ERROR: 403",
			"WARNING: [youtube] C: missing url",
			"ERROR: 403",
		})
		want := strings.Join([]string{
			"WARNING: [youtube] A: missing url",
			"ERROR: 403 (×3)",
			"WARNING: [youtube] B: missing url",
			"WARNING: [youtube] C: missing url",
		}, "\n")
		assert.Equal(t, want, got)
	})

	t.Run("oversized output is trimmed by line from the start", func(t *testing.T) {
		long := strings.Repeat("x", 800)
		got := summarizeIssues([]string{
			"OLD: " + long,
			"NEW: " + long,
		})
		assert.LessOrEqual(t, len(got), maxNotifyBytes+len("…\n"))
		assert.True(t, strings.HasPrefix(got, "…\n"), "expected ellipsis prefix when trimming, got: %q", got[:10])
		assert.Contains(t, got, "NEW: ")
		assert.NotContains(t, got, "OLD: ")
	})
}
