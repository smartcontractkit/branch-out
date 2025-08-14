package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeQuarantineTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		quarantineTargets []QuarantineTarget
		expected          []QuarantineTarget
	}{
		{
			name: "no duplicates",
			quarantineTargets: []QuarantineTarget{
				{Package: "github.com/example/pkg", Tests: []TestToQuarantine{
					{Name: "TestA", JiraTicket: "JIRA-A"},
					{Name: "TestB", JiraTicket: "JIRA-B"},
				}},
				{Package: "github.com/example/pkg", Tests: []TestToQuarantine{
					{Name: "TestB", JiraTicket: "JIRA-B"},
					{Name: "TestC", JiraTicket: "JIRA-C"},
				}},
			},
			expected: []QuarantineTarget{
				{
					Package: "github.com/example/pkg",
					Tests: []TestToQuarantine{
						{Name: "TestA", JiraTicket: "JIRA-A"},
						{Name: "TestB", JiraTicket: "JIRA-B"},
						{Name: "TestC", JiraTicket: "JIRA-C"},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			actual := sanitizeQuarantineTargets(test.quarantineTargets)
			assert.Equal(t, test.expected, actual)
		})
	}
}
