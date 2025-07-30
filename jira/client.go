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
	"time"

	go_jira "github.com/andygrunwald/go-jira"
	"github.com/rs/zerolog"
	"github.com/trivago/tgo/tcontainer"
	"golang.org/x/oauth2"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/telemetry"
	"github.com/smartcontractkit/branch-out/trunk"
)

// issueService defines the interface for go-jira issue operations.
// This is used for mocking in tests.
type issueService interface {
	Create(issue *go_jira.Issue) (*go_jira.Issue, *go_jira.Response, error)
	Update(issue *go_jira.Issue) (*go_jira.Issue, *go_jira.Response, error)
	Search(jql string, options *go_jira.SearchOptions) ([]go_jira.Issue, *go_jira.Response, error)
	AddComment(issueID string, comment *go_jira.Comment) (*go_jira.Comment, *go_jira.Response, error)
	GetTransitions(issueID string) ([]go_jira.Transition, *go_jira.Response, error)
	DoTransition(issueID, transitionID string) (*go_jira.Response, error)
}

// fieldService defines the interface for go-jira field operations.
// This is used for mocking in tests.
type fieldService interface {
	GetList() ([]go_jira.Field, *go_jira.Response, error)
}

const (
	// BranchOutLabel is the label used for any issues created by branch-out.
	BranchOutLabel = "branch-out"
	// FlakyTestLabel is the label used for any issues referencing a flaky test.
	FlakyTestLabel = "flaky-test"
)

var (
	// ErrBaseDomainRequired is returned when a base domain is required but not provided.
	ErrBaseDomainRequired = errors.New("jira base domain is required")
	// ErrProjectKeyRequired is returned when a project key is required but not provided.
	ErrProjectKeyRequired = errors.New("jira project key is required")
	// ErrNoAuthCredentialsProvided is returned when no authentication credentials are provided.
	ErrNoAuthCredentialsProvided = errors.New("no authentication credentials provided")

	// ErrFailedToAddCustomFields is returned when custom fields cannot be added to a Jira issue.
	ErrFailedToAddCustomFields = errors.New("failed to add custom fields to Jira issue")
	// ErrFailedToCreateFlakyTestIssue is returned when a flaky test issue cannot be created.
	ErrFailedToCreateFlakyTestIssue = errors.New("failed to create flaky test issue")
	// ErrNoOpenFlakyTestIssueFound is returned when searching for a specific flaky test issue and it is not found.
	ErrNoOpenFlakyTestIssueFound = errors.New("no open flaky test issue found")
	// ErrCustomFieldsNotFound is returned when the provided custom fields are not found in Jira.
	ErrCustomFieldsNotFound = errors.New("custom Jira fields not found")
	// ErrNoCloseTransition is returned when no close transition is available for an issue.
	ErrNoCloseTransition = errors.New("no close transition available")
)

// JiraAPIError provides detailed context about API failures for better caller logging
type JiraAPIError struct {
	Operation   string // The operation being performed (e.g., "create_issue", "add_comment")
	IssueKey    string // The issue key being operated on (if applicable)
	StatusCode  int    // HTTP status code from the API response
	APIResponse string // Raw API response body for debugging
	Underlying  error  // The underlying error that caused the failure
}

func (e *JiraAPIError) Error() string {
	if e.IssueKey != "" {
		return fmt.Sprintf("jira %s operation failed for issue %s (status %d): %v",
			e.Operation, e.IssueKey, e.StatusCode, e.Underlying)
	}
	return fmt.Sprintf("jira %s operation failed (status %d): %v",
		e.Operation, e.StatusCode, e.Underlying)
}

func (e *JiraAPIError) Unwrap() error {
	return e.Underlying
}

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

	// Services for mocking in tests
	IssueService issueService
	FieldService fieldService

	config  config.Jira
	logger  zerolog.Logger
	metrics *telemetry.Metrics

	jqlBase string
}

// Option is a function that sets a configuration option for the Jira client.
type Option func(*jiraClientOptions)

type jiraClientOptions struct {
	config       config.Jira
	logger       zerolog.Logger
	metrics      *telemetry.Metrics
	issueService issueService
	fieldService fieldService
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

// WithServices sets custom Issue and Field services.
// Handy for testing.
func WithServices(issueService issueService, fieldService fieldService) Option {
	return func(opts *jiraClientOptions) {
		opts.issueService = issueService
		opts.fieldService = fieldService
	}
}

// WithMetrics sets the metrics instance for the Jira client.
func WithMetrics(metrics *telemetry.Metrics) Option {
	return func(opts *jiraClientOptions) {
		opts.metrics = metrics
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
		return nil, ErrBaseDomainRequired
	}

	if jiraConfig.ProjectKey == "" {
		return nil, ErrProjectKeyRequired
	}

	// Check if proper authentication credentials are provided
	hasOAuth := jiraConfig.OAuthClientID != "" && jiraConfig.OAuthClientSecret != "" &&
		jiraConfig.OAuthAccessToken != ""
	hasBasicAuth := jiraConfig.Username != "" && jiraConfig.Token != ""

	if !hasOAuth && !hasBasicAuth {
		return nil, ErrNoAuthCredentialsProvided
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

	switch {
	case hasOAuth:
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
	case hasBasicAuth:
		httpClient = base.NewClient(
			"jira",
			base.WithLogger(l),
			base.WithBasicAuth(jiraConfig.Username, jiraConfig.Token),
		)
	default:
		return nil, ErrNoAuthCredentialsProvided
	}

	jiraClient, err := go_jira.NewClient(httpClient, baseURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}
	c := &Client{
		Client:  jiraClient,
		config:  jiraConfig,
		logger:  l,
		metrics: opts.metrics,
	}

	// Use injected services if provided, otherwise use real services
	if opts.issueService != nil && opts.fieldService != nil {
		c.IssueService = opts.issueService
		c.FieldService = opts.fieldService
	} else {
		c.IssueService = jiraClient.Issue
		c.FieldService = jiraClient.Field
	}

	c.logger = c.logger.With().Str("auth_type", c.AuthType()).Logger()

	err = c.validateCustomFields()
	if errors.Is(err, ErrCustomFieldsNotFound) {
		c.logger.Debug().
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
	ProjectKey        string `json:"project_key"`
	RepoURL           string `json:"repo_url"`
	Package           string `json:"package"`
	Test              string `json:"test"`
	FilePath          string `json:"file_path"`
	TrunkID           string `json:"trunk_id"`           // UUID from Trunk.io
	AdditionalDetails string `json:"additional_details"` // JSON string with additional details (trunk Payload for example)
}

// toJiraIssue converts a FlakyTestIssueRequest to a Jira issue.
func (f FlakyTestIssueRequest) toJiraIssue() *go_jira.Issue {
	summary := fmt.Sprintf("Flaky Test: %s.%s", f.Package, f.Test)

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
		f.RepoURL,
		f.Package,
		f.Test,
		f.FilePath,
		f.TrunkID,
		f.AdditionalDetails)
	return &go_jira.Issue{
		Fields: &go_jira.IssueFields{
			Project: go_jira.Project{
				Key: f.ProjectKey,
			},
			Summary:     summary,
			Description: description,
			Type: go_jira.IssueType{
				Name: "Bug",
			},
			Labels: []string{FlakyTestLabel, "automated", BranchOutLabel},
		},
	}
}

// CreateFlakyTestIssue creates a new Jira issue for a flaky test
func (c *Client) CreateFlakyTestIssue(req FlakyTestIssueRequest) (FlakyTestIssue, error) {
	ctx := context.Background()

	c.logger.Debug().
		Str("repo_url", req.RepoURL).
		Str("package", req.Package).
		Str("test", req.Test).
		Str("file_path", req.FilePath).
		Str("trunk_id", req.TrunkID).
		Msg("Creating Jira issue for flaky test")

	createStart := time.Now()
	issue, resp, err := c.IssueService.Create(req.toJiraIssue())
	if err != nil {
		c.metrics.RecordJiraAPILatency(ctx, "create_issue", time.Since(createStart))
		c.metrics.IncJiraTicket(ctx, "create_failed")
		return FlakyTestIssue{}, fmt.Errorf("%w: %w", ErrFailedToCreateFlakyTestIssue, err)
	}
	if err := checkResponse(resp); err != nil {
		c.metrics.RecordJiraAPILatency(ctx, "create_issue", time.Since(createStart))
		return FlakyTestIssue{}, err
	}
	c.metrics.RecordJiraAPILatency(ctx, "create_issue", time.Since(createStart))

	// Create the issue first, then try to add custom fields
	err = c.addCustomFields(issue.Key, req)
	if err != nil {
		c.logger.Debug().
			Err(err).
			Str("issue_key", issue.Key).
			Msg("Failed to add custom fields to Jira issue")
	}

	flakyTestIssue := c.wrapFlakyTestIssue(issue)

	// Record success metrics
	c.metrics.IncJiraTicket(ctx, "created")
	c.logger.Debug().
		Str("ticket_key", flakyTestIssue.Key).
		Str("ticket_id", flakyTestIssue.ID).
		Str("ticket_url", flakyTestIssue.Self).
		Msg("Created Jira issue for flaky test")

	return flakyTestIssue, nil
}

// addCustomFields updates an existing flaky test issue with custom fields
func (c *Client) addCustomFields(issueKey string, req FlakyTestIssueRequest) error {
	customFields := c.buildCustomFields(req)
	if len(customFields) == 0 {
		return nil // No custom fields configured
	}

	updateRequest := &go_jira.Issue{
		Key:    issueKey,
		Fields: &go_jira.IssueFields{Unknowns: customFields},
	}

	_, resp, err := c.IssueService.Update(updateRequest)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToAddCustomFields, err)
	}
	if err := checkResponse(resp); err != nil {
		return err
	}

	c.logger.Debug().
		Str("issue_key", issueKey).
		Int("field_count", len(customFields)).
		Msg("Updated Jira issue with custom fields")

	return nil
}

// buildCustomFields creates a map of custom fields from the request
func (c *Client) buildCustomFields(req FlakyTestIssueRequest) tcontainer.MarshalMap {
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

	return customFields
}

// GetOpenFlakyTestIssues returns all open flaky test tickets.
func (c *Client) GetOpenFlakyTestIssues() ([]FlakyTestIssue, error) {
	jql := fmt.Sprintf(
		`project = "%s" AND labels = "%s" AND status != "Closed"`,
		c.config.ProjectKey,
		FlakyTestLabel,
	)
	c.logger.Debug().Str("jql", jql).Msg("Searching for all open flaky test issues")
	issues, resp, err := c.IssueService.Search(
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
func (c *Client) GetOpenFlakyTestIssue(packageName, testName string) (FlakyTestIssue, error) {
	if packageName == "" || testName == "" {
		return FlakyTestIssue{}, fmt.Errorf("package name and test name are required")
	}

	searchFields := []string{"key", "id", "self"}

	// Try custom fields first (if configured), then fall back to summary search
	issue, err := c.getFlakyTestIssueByCustomFields(packageName, testName, searchFields)
	if errors.Is(err, ErrNoOpenFlakyTestIssueFound) {
		issue, err = c.getFlakyTestIssueBySummary(packageName, testName, searchFields)
	}
	if err != nil {
		return FlakyTestIssue{}, err
	}

	c.logger.Debug().
		Str("issue_key", issue.Key).
		Str("test", issue.Test).
		Str("package", issue.Package).
		Msg("Found open flaky test issue")

	return *issue, nil
}

// GetProjectKey returns the project key for the Jira client.
// Note: this is likely to be deprecated in the future as we'll be using multiple projects in Jira.
func (c *Client) GetProjectKey() string {
	return c.config.ProjectKey
}

// searchFlakyTestIssues performs a JQL search and returns the first matching issue
func (c *Client) searchFlakyTestIssues(jql string, searchFields []string, searchType string) (*FlakyTestIssue, error) {
	c.logger.Debug().
		Str("jql", jql).
		Str("search_type", searchType).
		Msg("Searching for flaky test issue")

	issues, resp, err := c.IssueService.Search(jql, &go_jira.SearchOptions{
		Fields: searchFields,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search for flaky test tickets: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	if len(issues) == 0 {
		return nil, ErrNoOpenFlakyTestIssueFound
	}
	if len(issues) > 1 {
		c.logger.Debug().
			Int("num_issues", len(issues)).
			Str("search_type", searchType).
			Msg("Multiple open flaky test issues found, returning the first one")
	}

	issue := c.wrapFlakyTestIssue(&issues[0])
	return &issue, nil
}

// getFlakyTestIssueByCustomFields searches for a flaky test issue by custom fields, if they're configured.
func (c *Client) getFlakyTestIssueByCustomFields(
	packageName, testName string,
	searchFields []string,
) (*FlakyTestIssue, error) {
	if c.config.TestFieldID == "" || c.config.PackageFieldID == "" {
		return nil, ErrNoOpenFlakyTestIssueFound // Return the standard "not found" error so caller can fallback
	}

	// Convert custom field IDs to numbers for JQL
	testFieldIDNum, err := strconv.Atoi(strings.TrimPrefix(c.config.TestFieldID, "customfield_"))
	if err != nil {
		return nil, fmt.Errorf("invalid test field ID format: %w", err)
	}
	packageFieldIDNum, err := strconv.Atoi(strings.TrimPrefix(c.config.PackageFieldID, "customfield_"))
	if err != nil {
		return nil, fmt.Errorf("invalid package field ID format: %w", err)
	}

	jql := fmt.Sprintf(`%s AND cf[%d] ~ "%s" AND cf[%d] ~ "%s" AND status != "Closed"`,
		c.jqlBase, testFieldIDNum, testName, packageFieldIDNum, packageName,
	)

	//nolint:gocritic // we don't want to modify the underlying slice
	enhancedFields := append(searchFields, c.config.TestFieldID, c.config.PackageFieldID)
	return c.searchFlakyTestIssues(jql, enhancedFields, "custom_fields")
}

// getFlakyTestIssueBySummary searches for a flaky test issue by the summary.
// This is a fallback for when the custom fields are not configured.
func (c *Client) getFlakyTestIssueBySummary(
	packageName, testName string,
	searchFields []string,
) (*FlakyTestIssue, error) {
	jql := fmt.Sprintf(`%s AND summary ~ "%s.%s" AND status != "Closed"`,
		c.jqlBase, packageName, testName,
	)

	return c.searchFlakyTestIssues(jql, searchFields, "summary")
}

// AuthType returns the type of authentication being used
func (c *Client) AuthType() string {
	switch {
	case c.config.OAuthAccessToken != "":
		return "OAuth"
	case c.config.Username != "" && c.config.Token != "":
		return "Basic"
	default:
		return "None"
	}
}

// validateCustomFields validates that, if provided, the custom fields are available in Jira.
func (c *Client) validateCustomFields() error {
	if c.config.TestFieldID == "" && c.config.PackageFieldID == "" && c.config.TrunkIDFieldID == "" {
		return nil
	}

	fields, resp, err := c.FieldService.GetList()
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
	if resp == nil {
		return nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read Jira API error response body: %w", err)
		}
		return fmt.Errorf("jira API error (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// checkResponseWithContext checks the response and returns a JiraAPIError with context
func checkResponseWithContext(resp *go_jira.Response, operation, issueKey string) error {
	if resp == nil {
		return nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		apiResponse := ""
		if err == nil {
			apiResponse = string(body)
		}

		return &JiraAPIError{
			Operation:   operation,
			IssueKey:    issueKey,
			StatusCode:  resp.StatusCode,
			APIResponse: apiResponse,
			Underlying:  fmt.Errorf("HTTP %d", resp.StatusCode),
		}
	}
	return nil
}

// wrapFlakyTestIssue converts a Jira issue to a FlakyTestIssue, extracting custom fields if available.
func (c *Client) wrapFlakyTestIssue(issue *go_jira.Issue) FlakyTestIssue {
	if issue == nil || issue.Fields == nil {
		return FlakyTestIssue{Issue: issue}
	}

	flakyTestIssue := FlakyTestIssue{Issue: issue}

	// Extract from custom fields first
	c.extractCustomFields(&flakyTestIssue, issue.Fields.Unknowns)

	// If custom fields are empty, try parsing from summary
	if flakyTestIssue.Test == "" || flakyTestIssue.Package == "" {
		c.extractFromSummary(&flakyTestIssue, issue.Fields.Summary)
	}

	return flakyTestIssue
}

// extractCustomFields extracts test, package, and trunk ID from custom fields
func (c *Client) extractCustomFields(issue *FlakyTestIssue, unknowns map[string]any) {
	if unknowns == nil {
		return
	}

	if c.config.TestFieldID != "" {
		if val := unknowns[c.config.TestFieldID]; val != nil {
			issue.Test = fmt.Sprint(val)
		}
	}
	if c.config.PackageFieldID != "" {
		if val := unknowns[c.config.PackageFieldID]; val != nil {
			issue.Package = fmt.Sprint(val)
		}
	}
	if c.config.TrunkIDFieldID != "" {
		if val := unknowns[c.config.TrunkIDFieldID]; val != nil {
			issue.TrunkID = fmt.Sprint(val)
		}
	}
}

// extractFromSummary parses test and package from issue summary as fallback
// Expected format: "Flaky Test: github.com/smartcontractkit/branch-out/package.TestName"
func (c *Client) extractFromSummary(issue *FlakyTestIssue, summary string) {
	summary = strings.TrimPrefix(summary, "Flaky Test: ")

	if lastDot := strings.LastIndex(summary, "."); lastDot != -1 {
		if issue.Package == "" {
			issue.Package = summary[:lastDot]
		}
		if issue.Test == "" {
			issue.Test = summary[lastDot+1:]
		}
	}
}

// callAndCheck performs a Jira API call and standardizes error handling
func (c *Client) callAndCheck(operation, issueKey string, call func() (*go_jira.Response, error)) error {
	resp, err := call()
	if err != nil {
		return &JiraAPIError{
			Operation:  operation,
			IssueKey:   issueKey,
			StatusCode: 0, // Unknown since the request failed
			Underlying: err,
		}
	}
	return checkResponseWithContext(resp, operation, issueKey)
}

// AddCommentToIssue adds a comment to an issue in Jira.
func (c *Client) AddCommentToIssue(issueKey, commentBody string) error {
	if issueKey == "" {
		return fmt.Errorf("issue key is required")
	}
	if commentBody == "" {
		return fmt.Errorf("comment body is required")
	}

	comment := &go_jira.Comment{
		Body: commentBody,
	}

	c.logger.Debug().
		Str("issue_key", issueKey).
		Int("comment_length", len(commentBody)).
		Msg("Adding comment to Jira issue")

	err := c.callAndCheck("add_comment", issueKey, func() (*go_jira.Response, error) {
		_, resp, err := c.IssueService.AddComment(issueKey, comment)
		return resp, err
	})
	if err != nil {
		return err
	}

	c.logger.Debug().
		Str("issue_key", issueKey).
		Msg("Successfully added comment to Jira issue")

	return nil
}

// flakyCommentData holds the data needed to build a flaky test status comment
type flakyCommentData struct {
	Emoji           string
	Title           string
	BodyIntro       string
	Previous        string
	Current         string
	FailureRate     float64
	PRCount         int
	TestSuite       string
	Variant         string
	URL             string
	AdditionalNotes string
}

// buildFlakyTestComment creates a formatted comment for flaky test status updates
func (c *Client) buildFlakyTestComment(data flakyCommentData) string {
	comment := fmt.Sprintf(`*Test Status Update: %s* %s

%s

*Status Change:* %s → %s
*Failure Rate (Last 7d):* %g%%
*Pull Requests Impacted (Last 7d):* %d`,
		data.Title,
		data.Emoji,
		data.BodyIntro,
		data.Previous,
		data.Current,
		data.FailureRate,
		data.PRCount,
	)

	// Add test suite and variant if available
	if data.TestSuite != "" {
		comment += fmt.Sprintf("\n*Test Suite:* %s", data.TestSuite)
	}
	if data.Variant != "" {
		comment += fmt.Sprintf("\n*Variant:* %s", data.Variant)
	}

	// Add URL and additional notes
	comment += fmt.Sprintf("\n*Test URL:* %s", data.URL)

	if data.AdditionalNotes != "" {
		comment += fmt.Sprintf("\n\n%s", data.AdditionalNotes)
	}

	comment += "\n\nThis comment was automatically added by [branch-out|https://github.com/smartcontractkit/branch-out]."

	return comment
}

// AddCommentToFlakyTestIssue adds a status-specific comment to an existing (open) flaky test issue
func (c *Client) AddCommentToFlakyTestIssue(issue FlakyTestIssue, statusChange trunk.TestCaseStatusChange) error {
	testCase := statusChange.TestCase
	currentStatus := statusChange.StatusChange.CurrentStatus.Value
	previousStatus := statusChange.StatusChange.PreviousStatus

	// Build comment data based on status
	data := flakyCommentData{
		Previous:    previousStatus,
		Current:     currentStatus,
		FailureRate: testCase.FailureRateLast7D,
		PRCount:     testCase.PullRequestsImpactedLast7D,
		TestSuite:   testCase.TestSuite,
		Variant:     testCase.Variant,
		URL:         testCase.HTMLURL,
	}

	switch currentStatus {
	case trunk.TestCaseStatusHealthy:
		data.Emoji = "✅"
		data.Title = "HEALTHY"
		data.BodyIntro = "The test has recovered and is now healthy!"
		data.AdditionalNotes = "This ticket should be closed as the test appears to be stable again."
	case trunk.TestCaseStatusFlaky:
		data.Emoji = "⚠️"
		data.Title = "FLAKY"
		data.BodyIntro = "Another flaky occurrence has been detected for this test."
	case trunk.TestCaseStatusBroken:
		data.Emoji = "❌"
		data.Title = "BROKEN"
		data.BodyIntro = "The test status has changed to broken."
	default:
		data.Emoji = ""
		data.Title = strings.ToUpper(currentStatus)
		data.BodyIntro = "The test status has been updated."
	}

	comment := c.buildFlakyTestComment(data)

	// Add the comment using the existing method
	err := c.AddCommentToIssue(issue.Key, comment)
	if err != nil {
		return fmt.Errorf("failed to add status comment to flaky test issue %s: %w", issue.Key, err)
	}

	c.logger.Debug().
		Str("issue_key", issue.Key).
		Str("status", currentStatus).
		Str("test_case_id", testCase.ID).
		Str("test_suite", testCase.TestSuite).
		Str("variant", testCase.Variant).
		Msg("Added status update comment to flaky test issue")

	return nil
}

// CloseIssue closes a Jira issue by transitioning it to a "Closed" status
func (c *Client) CloseIssue(issueKey, closeComment string) error {
	if issueKey == "" {
		return fmt.Errorf("issue key is required")
	}

	c.logger.Debug().
		Str("issue_key", issueKey).
		Bool("has_comment", closeComment != "").
		Msg("Closing Jira issue")

	// Track comment error separately
	var commentErr error

	// Add a comment explaining why the issue is being closed
	if closeComment != "" {
		commentErr = c.AddCommentToIssue(issueKey, closeComment)
		if commentErr != nil {
			c.logger.Warn().
				Str("issue_key", issueKey).
				Err(commentErr).
				Msg("Failed to add closing comment (continuing with transition)")
			// Don't fail the whole operation if comment fails
		}
	}

	// Get available transitions for this issue
	transitions, resp, err := c.IssueService.GetTransitions(issueKey)
	if err != nil {
		return &JiraAPIError{
			Operation:  "get_transitions",
			IssueKey:   issueKey,
			StatusCode: 0,
			Underlying: err,
		}
	}
	if err := checkResponseWithContext(resp, "get_transitions", issueKey); err != nil {
		return err
	}

	// Find a transition that leads to a "Closed" or "Done" status
	var closeTransition *go_jira.Transition
	for _, transition := range transitions {
		if transition.To.Name != "" && (strings.ToLower(transition.To.Name) == "closed" ||
			strings.ToLower(transition.To.Name) == "done" ||
			strings.ToLower(transition.To.Name) == "resolved") {
			closeTransition = &transition
			break
		}
	}

	if closeTransition == nil {
		c.logger.Debug().
			Str("issue_key", issueKey).
			Int("available_transitions", len(transitions)).
			Msg("No close/done/resolved transition found")
		return fmt.Errorf("%w for issue %s", ErrNoCloseTransition, issueKey)
	}

	// Execute the transition
	err = c.callAndCheck("do_transition", issueKey, func() (*go_jira.Response, error) {
		return c.IssueService.DoTransition(issueKey, closeTransition.ID)
	})
	if err != nil {
		return err
	}

	c.logger.Debug().
		Str("issue_key", issueKey).
		Str("transition_name", closeTransition.Name).
		Bool("comment_failed", commentErr != nil).
		Msg("Successfully closed Jira issue")

	return nil
}
