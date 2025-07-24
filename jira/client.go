// Package jira provides utilities for the Jira API.
package jira

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	go_jira "github.com/andygrunwald/go-jira"
	"github.com/rs/zerolog"
	"golang.org/x/oauth2"

	"github.com/smartcontractkit/branch-out/base"
	"github.com/smartcontractkit/branch-out/config"
)

// Client wraps the go-jira Client and provides some common methods.
type Client struct {
	*go_jira.Client

	config config.Jira
	logger zerolog.Logger
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

	if err := c.validateCustomFields(); err != nil {
		c.logger.Warn().
			Err(err).
			Msg("Provided custom field IDs are not available in Jira, some functionality will be disabled")
		c.config.TestFieldID = ""
		c.config.PackageFieldID = ""
		c.config.TrunkIDFieldID = ""
	}

	return c, nil
}

// FlakyTestIssueRequest represents the data needed to create a Jira ticket for a flaky test
type FlakyTestIssueRequest struct {
	RepoURL           string `json:"repo_url"`
	Package           string `json:"package"`
	Test              string `json:"test"`
	FilePath          string `json:"file_path"`
	TrunkID           string `json:"trunk_id"`           // UUID from Trunk.io
	AdditionalDetails string `json:"additional_details"` // JSON string with additional details (trunk Payload for example)
}

// CreateFlakyTestIssue creates a new Jira ticket for a flaky test
func (c *Client) CreateFlakyTestIssue(req FlakyTestIssueRequest) (*go_jira.Issue, error) {
	c.logger.Debug().
		Str("repo_url", req.RepoURL).
		Str("package", req.Package).
		Str("test", req.Test).
		Str("file_path", req.FilePath).
		Str("trunk_id", req.TrunkID).
		Msg("Creating Jira ticket for flaky test")

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
			Labels: []string{"flaky-test", "automated", "branch-out"},
		},
	}

	// If we have them, add the custom fields to the issue request
	if c.config.TestFieldID != "" {
		createIssueRequest.Fields.Unknowns[c.config.TestFieldID] = req.Test
	}
	if c.config.PackageFieldID != "" {
		createIssueRequest.Fields.Unknowns[c.config.PackageFieldID] = req.Package
	}
	if c.config.TrunkIDFieldID != "" {
		createIssueRequest.Fields.Unknowns[c.config.TrunkIDFieldID] = req.TrunkID
	}

	ticket, resp, err := c.Issue.Create(createIssueRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira issue: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}

	c.logger.Info().
		Str("ticket_key", ticket.Key).
		Str("ticket_id", ticket.ID).
		Str("ticket_url", ticket.Self).
		Msg("Created Jira ticket for flaky test")

	return ticket, nil
}

// GetOpenFlakyTestIssues returns all open flaky test tickets.
func (c *Client) GetOpenFlakyTestIssues() ([]go_jira.Issue, error) {
	jql := fmt.Sprintf(
		`project = "%s" AND labels = "branch-out" AND status != "Closed"`,
		c.config.ProjectKey,
	)
	c.logger.Debug().Str("jql", jql).Msg("Searching for all open flaky test tickets")
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
	c.logger.Debug().Int("num_issues", len(issues)).Msg("Finished searching for open flaky test tickets")
	return issues, nil
}

// GetOpenFlakyTestIssue returns the open flaky test ticket for a given package and test.
// If no ticket is found, it returns nil.
func (c *Client) GetOpenFlakyTestIssue(packageName, testName string) (*go_jira.Issue, error) {
	if packageName == "" || testName == "" {
		return nil, fmt.Errorf("package name and test name are required")
	}

	jql := fmt.Sprintf(
		`project = "%s" AND labels = "branch-out" AND summary ~ "%s.%s" AND status != "Closed"`,
		c.config.ProjectKey,
		packageName,
		testName,
	)
	// If we have the custom fields, use them to filter the results instead of the summary
	if c.config.TestFieldID != "" && c.config.PackageFieldID != "" {
		jql = fmt.Sprintf(
			`project = "%s" AND labels = "branch-out" AND %s = "%s" AND %s = "%s" AND status != "Closed"`,
			c.config.ProjectKey,
			c.config.TestFieldID,
			testName,
			c.config.PackageFieldID,
			packageName,
		)
	}
	c.logger.Debug().
		Str("package_name", packageName).
		Str("test_name", testName).
		Str("jql", jql).
		Msg("Searching for open flaky test ticket")
	issues, resp, err := c.Issue.Search(
		jql,
		&go_jira.SearchOptions{
			Fields: []string{"key", "id", "self"},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get Jira issue: %w", err)
	}
	if err := checkResponse(resp); err != nil {
		return nil, err
	}
	if len(issues) == 0 {
		c.logger.Debug().Msg("No open flaky test ticket found")
		return nil, nil // No open flaky test ticket found
	}
	issue := issues[0]
	c.logger.Debug().
		Str("ticket_key", issue.Key).
		Str("ticket_id", issue.ID).
		Str("ticket_url", issue.Self).
		Msg("Found open flaky test ticket")
	return &issue, nil // Return the first open flaky test ticket
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

// validateCustomFields validates that the custom fields are available in Jira.
func (c *Client) validateCustomFields() error {
	if c.config.TestFieldID == "" && c.config.PackageFieldID == "" && c.config.TrunkIDFieldID == "" {
		c.logger.Debug().Msg("No custom fields configured")
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
		return fmt.Errorf("unable to find all provided custom field IDs in Jira, found IDs for %v", foundFields)
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
