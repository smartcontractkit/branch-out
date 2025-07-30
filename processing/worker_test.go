package processing

import (
	"errors"
	"strings"
	"testing"

	go_jira "github.com/andygrunwald/go-jira"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/branch-out/internal/testhelpers"
	"github.com/smartcontractkit/branch-out/jira"
	"github.com/smartcontractkit/branch-out/trunk"
)

func TestWorker_HandleHealthyTest(t *testing.T) {
	t.Parallel()

	// Helper to create test case status changes with common fields
	makeStatusChange := func(current, previous string, withDetails bool) trunk.TestCaseStatusChange {
		testCase := trunk.TestCase{
			ID:        "test-123",
			Name:      "TestExample",
			TestSuite: "github.com/example/pkg",
		}

		if withDetails {
			testCase.FailureRateLast7D = 25.5
			testCase.PullRequestsImpactedLast7D = 3
			testCase.HTMLURL = "https://app.trunk.io/test/123"
		}

		return trunk.TestCaseStatusChange{
			TestCase: testCase,
			StatusChange: trunk.StatusChange{
				CurrentStatus:  trunk.Status{Value: current},
				PreviousStatus: previous,
			},
		}
	}

	testCases := []struct {
		name          string
		statusChange  trunk.TestCaseStatusChange
		setupMocks    func(*MockJiraClient)
		expectedError bool
		errorContains string
	}{
		{
			name:         "no existing ticket - should do nothing",
			statusChange: makeStatusChange(trunk.TestCaseStatusHealthy, trunk.TestCaseStatusFlaky, false),
			setupMocks: func(mockJira *MockJiraClient) {
				mockJira.EXPECT().GetOpenFlakyTestIssue("github.com/example/pkg", "TestExample").
					Return(jira.FlakyTestIssue{}, jira.ErrNoOpenFlakyTestIssueFound)
			},
			expectedError: false,
		},
		{
			name:         "previous status not flaky - should still check for existing tickets",
			statusChange: makeStatusChange(trunk.TestCaseStatusHealthy, trunk.TestCaseStatusHealthy, false),
			setupMocks: func(mockJira *MockJiraClient) {
				// Should still check for existing tickets even if previous status wasn't flaky
				mockJira.EXPECT().GetOpenFlakyTestIssue("github.com/example/pkg", "TestExample").
					Return(jira.FlakyTestIssue{}, jira.ErrNoOpenFlakyTestIssueFound)
			},
			expectedError: false,
		},
		{
			name:         "existing ticket found - should close it",
			statusChange: makeStatusChange(trunk.TestCaseStatusHealthy, trunk.TestCaseStatusFlaky, true),
			setupMocks: func(mockJira *MockJiraClient) {
				issue := jira.FlakyTestIssue{
					Issue: &go_jira.Issue{
						Key: "PROJ-123",
					},
				}
				mockJira.EXPECT().GetOpenFlakyTestIssue("github.com/example/pkg", "TestExample").
					Return(issue, nil)

				mockJira.EXPECT().CloseIssue("PROJ-123", mock.MatchedBy(func(comment string) bool {
					// Verify the comment contains expected content (using actual worker template text)
					return strings.Contains(comment, "Test Status Update: HEALTHY") &&
						strings.Contains(comment, "flaky → healthy") &&
						strings.Contains(comment, "25.5%") &&
						strings.Contains(comment, "3") &&
						strings.Contains(comment, "https://app.trunk.io/test/123") &&
						strings.Contains(comment, "Automatically Closing Ticket")
				})).Return(nil)
			},
			expectedError: false,
		},
		{
			name:         "error getting existing ticket - should fail",
			statusChange: makeStatusChange(trunk.TestCaseStatusHealthy, trunk.TestCaseStatusFlaky, false),
			setupMocks: func(mockJira *MockJiraClient) {
				mockJira.EXPECT().GetOpenFlakyTestIssue("github.com/example/pkg", "TestExample").
					Return(jira.FlakyTestIssue{}, errors.New("API error"))
			},
			expectedError: true,
			errorContains: "failed to check for existing Jira ticket",
		},
		{
			name:         "close ticket fails - should not fail overall operation",
			statusChange: makeStatusChange(trunk.TestCaseStatusHealthy, trunk.TestCaseStatusFlaky, true),
			setupMocks: func(mockJira *MockJiraClient) {
				issue := jira.FlakyTestIssue{
					Issue: &go_jira.Issue{
						Key: "PROJ-123",
					},
				}
				mockJira.EXPECT().GetOpenFlakyTestIssue("github.com/example/pkg", "TestExample").
					Return(issue, nil)

				mockJira.EXPECT().CloseIssue("PROJ-123", mock.Anything).
					Return(errors.New("close failed"))
			},
			expectedError: false, // Should not fail overall operation
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := testhelpers.Logger(t)
			mockJira := NewMockJiraClient(t)

			// Set up mock expectations
			tc.setupMocks(mockJira)

			// Ensure all mock expectations are verified
			t.Cleanup(func() {
				mockJira.AssertExpectations(t)
			})

			worker := NewWorker(
				logger,
				nil, // AWS client not used in handleHealthyTest
				mockJira,
				nil, // Trunk client not used in handleHealthyTest
				nil, // Github client not used in handleHealthyTest
				nil, // metrics
				Config{},
			)

			err := worker.handleHealthyTest(logger, tc.statusChange)

			if tc.expectedError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
