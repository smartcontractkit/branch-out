// Package jira provides utilities for the Jira API.
package jira

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	go_jira "github.com/andygrunwald/go-jira"
	"github.com/rs/zerolog"
	"github.com/trivago/tgo/tcontainer"
	"golang.org/x/oauth2"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
)

const (
	// BranchOutLabel is the label used for any issues created by branch-out.
	BranchOutLabel = "branch-out"
	// FlakyTestLabel is the label used for any issues referencing a flaky test.
	FlakyTestLabel = "flaky-test"
)

var (
	// ErrNoOpenFlakyTestIssueFound is returned when searching for a specific flaky test issue and it is not found.
	ErrNoOpenFlakyTestIssueFound = errors.New("no open flaky test issue found")
	// ErrCustomFieldsNotFound is returned when the provided custom fields are not found in Jira.
	ErrCustomFieldsNotFound = errors.New("custom Jira fields not found")
)

// FlakyTestIssue represents a Jira issue for a flaky test.
type FlakyTestIssue struct {
	*go_jira.Issue

	Test    string `json:"test"`
	Package string `json:"package"`
	TrunkID string `json:"trunk_id"`
}

// Client wraps the go-jira Client and provides some common methods.
type Client struct {
	*go_jira.Client

	config config.Jira
	logger zerolog.Logger

	jqlBase string
}

// Option is a function that sets a configuration option for the Jira client.
type Option func(*jiraClientOptions)

type jiraClientOptions struct {
	config config.Jira
	logger zerolog.Logger
}

// WithLogger sets the logger to use for the Jira client.
func WithLogger(logger zerolog.Logger) Option {
	return func(opts *jiraClientOptions) {
		opts.logger = logger
	}
}

// WithConfig sets the config to use for the Jira client.
func WithConfig(config config.Config) Option {
	return func(opts *jiraClientOptions) {
		opts.config = config.Jira
	}
}

// NewClient creates a new Jira client with configuration from environment variables
func NewClient(options ...Option) (*Client, error) {
	opts := &jiraClientOptions{
		logger: zerolog.Nop(),
	}
	for _, opt := range options {
		opt(opts)
	}

	jiraConfig := opts.config

	if jiraConfig.BaseDomain == "" {
		return nil, fmt.Errorf("jira base domain is required")
	}

	if jiraConfig.ProjectKey == "" {
		return nil, fmt.Errorf("jira project key is required")
	}

	// Check if proper authentication credentials are provided
	hasOAuth := jiraConfig.OAuthClientID != "" && jiraConfig.OAuthClientSecret != "" &&
		jiraConfig.OAuthAccessToken != ""
	hasBasicAuth := jiraConfig.Username != "" && jiraConfig.Token != ""

	if !hasOAuth && !hasBasicAuth {
		return nil, fmt.Errorf(
			"jira OAuth credentials or basic auth credentials are required",
		)
	}

	l := opts.logger.With().
		Str("jira_base_domain", jiraConfig.BaseDomain).
		Str("jira_project_key", jiraConfig.ProjectKey).
		Logger()

	var (
		baseURL = &url.URL{
			Scheme: "https",
			Host:   jiraConfig.BaseDomain,
		}
		oauthTokenURL = baseURL.JoinPath("plugins/servlet/oauth/access-token")
		httpClient    *http.Client
	)

	if hasOAuth {
		oauthConfig := &oauth2.Config{
			ClientID:     jiraConfig.OAuthClientID,
			ClientSecret: jiraConfig.OAuthClientSecret,
			Endpoint: oauth2.Endpoint{
				TokenURL: oauthTokenURL.String(),
			},
		}

		token := &oauth2.Token{
			AccessToken:  jiraConfig.OAuthAccessToken,
			RefreshToken: jiraConfig.OAuthRefreshToken,
			TokenType:    "Bearer",
		}

		// Use context to pass the base transport to the oauth2 client
		clientCtx := context.WithValue(
			context.Background(),
			oauth2.HTTPClient,
			base.NewTransport("jira", base.WithLogger(l)),
		)
		httpClient = oauthConfig.Client(clientCtx, token)
	} else if hasBasicAuth {
		httpClient = base.NewClient(
			"jira",
			base.WithLogger(l),
			base.WithBasicAuth(jiraConfig.Username, jiraConfig.Token),
		)
	}

	jiraClient, err := go_jira.NewClient(httpClient, baseURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}
	c := &Client{
		Client: jiraClient,
		config: jiraConfig,
		logger: l,
	}

	c.logger = c.logger.With().Str("auth_type", c.AuthType()).Logger()

	err = c.validateCustomFields()
	if errors.Is(err, ErrCustomFieldsNotFound) {
		c.logger.Warn().
			Err(err).
			Msg("Provided custom field IDs are not available in Jira, some functionality will be disabled")
		c.config.TestFieldID = ""
		c.config.PackageFieldID = ""
		c.config.TrunkIDFieldID = ""
	} else if err != nil {
		return nil, fmt.Errorf("failed to validate custom Jira fields: %w", err)
	}

	c.jqlBase = fmt.Sprintf(
		`project = "%s" AND labels = "%s" AND labels = "%s"`,
		c.config.ProjectKey,
		FlakyTestLabel,
		BranchOutLabel,
	)

	return c, nil
}

// FlakyTestIssueRequest represents the data needed to create a Jira issue for a flaky test
type FlakyTestIssueRequest struct {
	RepoURL           string `json:"repo_url"`
	Package           string `json:"package"`
	Test              string `json:"test"`
	FilePath          string `json:"file_path"`
	TrunkID           string `json:"trunk_id"`           // UUID from Trunk.io
	AdditionalDetails string `json:"additional_details"` // JSON string with additional details (trunk Payload for example)
}

// CreateFlakyTestIssue creates a new Jira issue for a flaky test
func (c *Client) CreateFlakyTestIssue(req FlakyTestIssueRequest) (FlakyTestIssue, error) {
	c.logger.Debug().
		Str("repo_url", req.RepoURL).
		Str("package", req.Package).
		Str("test", req.Test).
		Str("file_path", req.FilePath).
		Str("trunk_id", req.TrunkID).
		Msg("Creating Jira issue for flaky test")

	// Construct the ticket summary and description
	summary := fmt.Sprintf("Flaky Test: %s.%s", req.Package, req.Test)

	description := fmt.Sprintf(`*Flaky Test Detected*

*Repo:* %s
*Package:* %s
*Test:* %s
*File Path:* %s
*Trunk ID:* %s

*Additional Details:*
{code:json}
%s
{code}

This ticket was automatically created by [branch-out|https://github.com/smartcontractkit/branch-out].`,
		req.RepoURL,
		req.Package,
		req.Test,
		req.FilePath,
		req.TrunkID,
		req.AdditionalDetails)

	// Create the Jira issue request
	createIssueRequest := &go_jira.Issue{
		Fields: &go_jira.IssueFields{
			Project: go_jira.Project{
				Key: c.config.ProjectKey,
			},
			Summary:     summary,
			Description: description,
			Type: go_jira.IssueType{
				Name: "Bug",
			},
			Labels: []string{FlakyTestLabel, "automated", BranchOutLabel},
		},
	}

	issue, resp, err := c.Issue.Create(createIssueRequest)
	if err != nil {
		return FlakyTestIssue{}, fmt.Errorf("failed to create Jira issue: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return FlakyTestIssue{}, err
	}

	// Create the issue first, then try to add custom fields
	err = c.addCustomFields(issue.Key, req)
	if err != nil {
		c.logger.Warn().
			Err(err).
			Msg("Failed to add custom fields to Jira issue")
	}

	flakyTestIssue := c.wrapFlakyTestIssue(issue)

	c.logger.Info().
		Str("ticket_key", flakyTestIssue.Key).
		Str("ticket_id", flakyTestIssue.ID).
		Str("ticket_url", flakyTestIssue.Self).
		Msg("Created Jira issue for flaky test")

	return flakyTestIssue, nil
}

// addCustomFields updates an existing flaky test issue with custom fields
func (c *Client) addCustomFields(
	issueKey string,
	req FlakyTestIssueRequest,
) error {
	// Only add custom fields if they're configured
	if c.config.TestFieldID == "" && c.config.PackageFieldID == "" && c.config.TrunkIDFieldID == "" {
		return nil
	}

	customFields := tcontainer.NewMarshalMap()
	if c.config.TestFieldID != "" {
		customFields[c.config.TestFieldID] = req.Test
	}
	if c.config.PackageFieldID != "" {
		customFields[c.config.PackageFieldID] = req.Package
	}
	if c.config.TrunkIDFieldID != "" {
		customFields[c.config.TrunkIDFieldID] = req.TrunkID
	}

	updateRequest := &go_jira.Issue{
		Key: issueKey,
		Fields: &go_jira.IssueFields{
			Unknowns: customFields,
		},
	}

	_, resp, err := c.Issue.Update(updateRequest)
	if err != nil {
		return fmt.Errorf("failed to update Jira issue with custom fields: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return err
	}

	return nil
}

// GetOpenFlakyTestIssues returns all open flaky test tickets.
func (c *Client) GetOpenFlakyTestIssues() ([]FlakyTestIssue, error) {
	jql := fmt.Sprintf(
		`project = "%s" AND labels = "%s" AND status != "Closed"`,
		c.config.ProjectKey,
		FlakyTestLabel,
	)
	c.logger.Debug().Str("jql", jql).Msg("Searching for all open flaky test issues")
	issues, resp, err := c.Issue.Search(
		jql,
		&go_jira.SearchOptions{
			Fields: []string{"key", "id", "self"},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search for flaky test tickets: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	c.logger.Debug().Int("num_issues", len(issues)).Msg("Finished searching for open flaky test issues")

	flakyTestIssues := []FlakyTestIssue{}
	for _, issue := range issues {
		flakyTestIssue := c.wrapFlakyTestIssue(&issue)
		flakyTestIssues = append(flakyTestIssues, flakyTestIssue)
	}

	return flakyTestIssues, nil
}

// GetOpenFlakyTestIssue returns the open flaky test ticket for a given package and test.
// If no ticket is found, it returns nil.
func (c *Client) GetOpenFlakyTestIssue(packageName, testName string) (FlakyTestIssue, error) {
	if packageName == "" || testName == "" {
		return FlakyTestIssue{}, fmt.Errorf("package name and test name are required")
	}

	searchFields := []string{"key", "id", "self"}
	jql := fmt.Sprintf(
		`%s AND summary ~ "%s.%s" AND status != "Closed"`,
		c.jqlBase,
		packageName,
		testName,
	)
	// If we have the custom fields, use them to filter the results instead of the summary
	if c.config.TestFieldID != "" && c.config.PackageFieldID != "" {
		searchFields = append(searchFields, c.config.TestFieldID, c.config.PackageFieldID)
		// JQL uses numbers for custom field searches, and doesn't support exact string matches. e.g. cf[10003] ~ "TestName"
		// https://support.atlassian.com/jira-software-cloud/docs/jql-fields/
		testFieldIDNum, err := strconv.Atoi(strings.TrimPrefix(c.config.TestFieldID, "customfield_"))
		if err != nil {
			return FlakyTestIssue{}, fmt.Errorf("failed to convert test field ID to number: %w", err)
		}
		packageFieldIDNum, err := strconv.Atoi(strings.TrimPrefix(c.config.PackageFieldID, "customfield_"))
		if err != nil {
			return FlakyTestIssue{}, fmt.Errorf("failed to convert package field ID to number: %w", err)
		}
		jql = fmt.Sprintf(
			`%s AND cf[%d] ~ "%s" AND cf[%d] ~ "%s"`,
			c.jqlBase,
			testFieldIDNum,
			testName,
			packageFieldIDNum,
			packageName,
		)
	}
	c.logger.Debug().
		Str("package_name", packageName).
		Str("test_name", testName).
		Str("jql", jql).
		Msg("Searching for open flaky test issue")
	issues, resp, err := c.Issue.Search(
		jql,
		&go_jira.SearchOptions{
			Fields: searchFields,
		},
	)
	if err != nil {
		return FlakyTestIssue{}, fmt.Errorf("failed to get Jira issue: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return FlakyTestIssue{}, err
	}

	if len(issues) == 0 {
		return FlakyTestIssue{}, ErrNoOpenFlakyTestIssueFound
	}
	if len(issues) > 1 {
		c.logger.Warn().
			Int("num_issues", len(issues)).
			Msg("Multiple open flaky test issues found, returning the first one")
	}

	issue := c.wrapFlakyTestIssue(&issues[0])

	c.logger.Debug().
		Str("issue_key", issue.Key).
		Str("test", issue.Test).
		Str("package", issue.Package).
		Str("issue_id", issue.ID).
		Str("issue_url", issue.Self).
		Msg("Found open flaky test issue")
	return issue, nil
}

// AuthType returns the type of authentication being used
func (c *Client) AuthType() string {
	if c.config.OAuthAccessToken != "" {
		return "OAuth"
	} else if c.config.Username != "" && c.config.Token != "" {
		return "Basic Auth"
	}
	return "None"
}

// validateCustomFields validates that, if provided, the custom fields are available in Jira.
func (c *Client) validateCustomFields() error {
	if c.config.TestFieldID == "" && c.config.PackageFieldID == "" && c.config.TrunkIDFieldID == "" {
		return nil
	}

	fields, resp, err := c.Field.GetList()
	if err != nil {
		return fmt.Errorf("failed to get custom field options: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return err
	}

	foundFields := []string{}
	for _, field := range fields {
		if field.ID == c.config.TestFieldID {
			foundFields = append(foundFields, "Test")
		}
		if field.ID == c.config.PackageFieldID {
			foundFields = append(foundFields, "Package")
		}
		if field.ID == c.config.TrunkIDFieldID {
			foundFields = append(foundFields, "Trunk ID")
		}
	}

	if len(foundFields) < 3 {
		return ErrCustomFieldsNotFound
	}

	return nil
}

// checkResponse checks the response from the Jira API and returns an error if the status code is not a success.
func checkResponse(resp *go_jira.Response) error {
	if resp != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read Jira API error response body: %w", err)
			}
			return fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(body))
		}
	}
	return nil
}

// wrapFlakyTestIssue converts a Jira issue to a FlakyTestIssue, extracting custom fields if available.
func (c *Client) wrapFlakyTestIssue(issue *go_jira.Issue) FlakyTestIssue {
	if issue == nil {
		return FlakyTestIssue{}
	}

	flakyTestIssue := FlakyTestIssue{
		Issue: issue,
	}

	if issue.Fields == nil {
		return flakyTestIssue
	}

	if issue.Fields.Unknowns != nil {
		if c.config.TestFieldID != "" {
			flakyTestIssue.Test = fmt.Sprint(issue.Fields.Unknowns[c.config.TestFieldID])
		}
		if c.config.PackageFieldID != "" {
			flakyTestIssue.Package = fmt.Sprint(issue.Fields.Unknowns[c.config.PackageFieldID])
		}
		if c.config.TrunkIDFieldID != "" {
			flakyTestIssue.TrunkID = fmt.Sprint(issue.Fields.Unknowns[c.config.TrunkIDFieldID])
		}
	}

	// If we don't have the fields, try to get it from the summary
	// Example: "Flaky Test: github.com/smartcontractkit/branch-out/package.TestName"
	if flakyTestIssue.Test == "" || flakyTestIssue.Package == "" {
		summary := issue.Fields.Summary
		summary = strings.TrimPrefix(summary, "Flaky Test: ")

		// Find the last dot to split package from test name
		if lastDot := strings.LastIndex(summary, "."); lastDot != -1 {
			if flakyTestIssue.Package == "" {
				flakyTestIssue.Package = summary[:lastDot]
			}
			if flakyTestIssue.Test == "" {
				flakyTestIssue.Test = summary[lastDot+1:]
			}
		}
	}

	return flakyTestIssue
}
