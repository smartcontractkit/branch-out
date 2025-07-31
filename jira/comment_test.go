package jira

import (
	"fmt"
	"strings"
	"testing"

	"github.com/smartcontractkit/branch-out/trunk"
	"github.com/stretchr/testify/assert"
)

func TestBuildFlakyTestComment(t *testing.T) {
	tests := []struct {
		name         string
		statusChange trunk.TestCaseStatusChange
	}{
		{
			name: "flaky to broken transition",
			statusChange: trunk.TestCaseStatusChange{
				TestCase: trunk.TestCase{
					FailureRateLast7D:          45.5,
					PullRequestsImpactedLast7D: 12,
					HTMLURL:                    "https://trunk.io/test/123",
				},
				StatusChange: trunk.StatusChange{
					CurrentStatus: trunk.Status{Value: "broken"},
					PreviousStatus: "flaky",
				},
			},
		},
		{
			name: "broken to healthy transition",
			statusChange: trunk.TestCaseStatusChange{
				TestCase: trunk.TestCase{
					FailureRateLast7D:          0.0,
					PullRequestsImpactedLast7D: 0,
					HTMLURL:                    "https://trunk.io/test/456",
				},
				StatusChange: trunk.StatusChange{
					CurrentStatus: trunk.Status{Value: "healthy"},
					PreviousStatus: "broken",
				},
			},
		},
		{
			name: "healthy to flaky transition",
			statusChange: trunk.TestCaseStatusChange{
				TestCase: trunk.TestCase{
					FailureRateLast7D:          23.1,
					PullRequestsImpactedLast7D: 5,
					HTMLURL:                    "https://trunk.io/test/789",
				},
				StatusChange: trunk.StatusChange{
					CurrentStatus: trunk.Status{Value: "flaky"},
					PreviousStatus: "healthy",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := buildFlakyTestComment(tt.statusChange)
			
			// Verify all data values are present (without strict formatting)
			assert.Contains(t, comment, strings.ToUpper(tt.statusChange.StatusChange.CurrentStatus.Value), "comment should contain current status")
			assert.Contains(t, comment, tt.statusChange.StatusChange.PreviousStatus, "comment should contain previous status")
			assert.Contains(t, comment, tt.statusChange.TestCase.HTMLURL, "comment should contain test URL")
			assert.Contains(t, comment, "branch-out", "comment should contain branch-out reference")
			
			// Verify numeric values are present (as strings)
			failureRateStr := strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%g", tt.statusChange.TestCase.FailureRateLast7D), ".0"), "0")
			assert.Contains(t, comment, failureRateStr, "comment should contain failure rate")
			
			prImpactedStr := fmt.Sprintf("%d", tt.statusChange.TestCase.PullRequestsImpactedLast7D)
			assert.Contains(t, comment, prImpactedStr, "comment should contain PR impact count")
			
			// Verify basic structure and formatting
			assert.NotEmpty(t, comment)
			assert.Greater(t, len(comment), 100, "comment should be reasonably detailed")
			assert.Contains(t, comment, "*", "comment should contain Markdown formatting")
		})
	}
}

func TestFormatFlakyTestComment(t *testing.T) {
	data := CommentData{
		CurrentStatus:              "flaky",
		PreviousStatus:             "healthy", 
		FailureRateLast7D:          15.5,
		PullRequestsImpactedLast7D: 3,
		TestURL:                    "https://trunk.io/test/example",
	}

	comment := formatFlakyTestComment(data)

	// Test that all data is present in the comment
	assert.Contains(t, comment, "FLAKY", "comment should contain uppercase current status")
	assert.Contains(t, comment, "healthy", "comment should contain previous status")
	assert.Contains(t, comment, "flaky", "comment should contain current status")
	assert.Contains(t, comment, "15.5", "comment should contain failure rate")
	assert.Contains(t, comment, "3", "comment should contain PR impact count")
	assert.Contains(t, comment, "https://trunk.io/test/example", "comment should contain test URL")
	assert.Contains(t, comment, "branch-out", "comment should contain branch-out reference")

	// Verify basic structure
	assert.NotEmpty(t, comment)
	assert.Contains(t, comment, "*", "comment should contain Markdown formatting")
	
	lines := strings.Split(comment, "\n")
	assert.Greater(t, len(lines), 5, "comment should have multiple lines")
}

func TestCommentConsistency(t *testing.T) {
	// Test that the same input always produces the same output
	statusChange := trunk.TestCaseStatusChange{
		TestCase: trunk.TestCase{
			FailureRateLast7D:          42.0,
			PullRequestsImpactedLast7D: 7,
			HTMLURL:                    "https://trunk.io/consistent",
		},
		StatusChange: trunk.StatusChange{
			CurrentStatus: trunk.Status{Value: "broken"},
			PreviousStatus: "flaky",
		},
	}

	comment1 := buildFlakyTestComment(statusChange)
	comment2 := buildFlakyTestComment(statusChange)

	assert.Equal(t, comment1, comment2, "same input should produce identical output")
}

func TestCommentDataEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		data CommentData
	}{
		{
			name: "zero values",
			data: CommentData{
				CurrentStatus:              "",
				PreviousStatus:             "",
				FailureRateLast7D:          0.0,
				PullRequestsImpactedLast7D: 0,
				TestURL:                    "",
			},
		},
		{
			name: "high failure rate",
			data: CommentData{
				CurrentStatus:              "broken",
				PreviousStatus:             "healthy",
				FailureRateLast7D:          100.0,
				PullRequestsImpactedLast7D: 999,
				TestURL:                    "https://very-long-url.example.com/with/many/path/segments/test/case/123456789",
			},
		},
		{
			name: "special characters in status",
			data: CommentData{
				CurrentStatus:              "test-status",
				PreviousStatus:             "another_status",
				FailureRateLast7D:          50.5,
				PullRequestsImpactedLast7D: 10,
				TestURL:                    "https://example.com/test?param=value&other=123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic and should return a string
			comment := formatFlakyTestComment(tt.data)
			assert.IsType(t, "", comment)
			
			// Should contain the data values
			if tt.data.CurrentStatus != "" {
				assert.Contains(t, comment, strings.ToUpper(tt.data.CurrentStatus))
			}
			if tt.data.PreviousStatus != "" {
				assert.Contains(t, comment, tt.data.PreviousStatus)
			}
			if tt.data.TestURL != "" {
				assert.Contains(t, comment, tt.data.TestURL)
			}
		})
	}
}
