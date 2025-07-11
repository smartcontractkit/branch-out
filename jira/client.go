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

// Client wraps the go-jira client and provides some common methods.
type Client struct {
	*go_jira.Client

	jiraConfig config.Jira
	logger     zerolog.Logger
}

// Option is a function that sets a configuration option for the Jira client.
type Option func(*jiraClientOptions)

type jiraClientOptions struct {
	jiraConfig config.Jira
	logger     zerolog.Logger
}

// WithLogger sets the logger to use for the Jira client.
func WithLogger(logger zerolog.Logger) Option {
	return func(cfg *jiraClientOptions) {
		cfg.logger = logger
	}
}

// WithConfig sets the config to use for the Jira client.
func WithConfig(config config.Config) Option {
	return func(cfg *jiraClientOptions) {
		cfg.jiraConfig = config.Jira
	}
}

// IClient is the interface that wraps the basic Jira client methods.
// Helpful for mocking in tests.
type IClient interface {
	CreateFlakyTestTicket(req FlakyTestTicketRequest) (*go_jira.Issue, error)
}

// NewClient creates a new Jira client with configuration from environment variables
func NewClient(options ...Option) (*Client, error) {
	opts := &jiraClientOptions{
		logger: zerolog.Nop(),
	}
	for _, opt := range options {
		opt(opts)
	}

	jiraConfig := opts.jiraConfig

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
		l = l.With().Str("auth_type", "Basic").Logger()
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
		Client:     jiraClient,
		jiraConfig: jiraConfig,
		logger:     l,
	}

	c.logger = c.logger.With().Str("auth_type", c.AuthType()).Logger()

	return c, nil
}

// FlakyTestTicketRequest represents the data needed to create a Jira ticket for a flaky test
type FlakyTestTicketRequest struct {
	RepoURL           string `json:"repo_url"`
	Package           string `json:"package"`
	Test              string `json:"test"`
	FilePath          string `json:"file_path"`
	TrunkID           string `json:"trunk_id"`           // UUID
	AdditionalDetails string `json:"additional_details"` // JSON string with additional details (trunk Payload for example)
}

// CreateFlakyTestTicket creates a new Jira ticket for a flaky test
func (c *Client) CreateFlakyTestTicket(req FlakyTestTicketRequest) (*go_jira.Issue, error) {
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

This ticket was automatically created by [branch-out](https://github.com/smartcontractkit/branch-out).`,
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
				Key: c.jiraConfig.ProjectKey,
			},
			Summary:     summary,
			Description: description,
			Type: go_jira.IssueType{
				Name: "Bug",
			},
			Labels: []string{"flaky-test", "automated", "branch-out"},
		},
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

// GetOpenFlakyTestTickets returns all open flaky test tickets.
func (c *Client) GetOpenFlakyTestTickets() ([]go_jira.Issue, error) {
	jql := fmt.Sprintf(
		`project = "%s" AND labels = "branch-out" AND status != "Closed"`,
		c.jiraConfig.ProjectKey,
	)
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
	return issues, nil
}

// GetOpenFlakyTestIssue returns the open flaky test ticket for a given package and test.
// If no ticket is found, it returns nil.
func (c *Client) GetOpenFlakyTestIssue(packageName, testName string) (*go_jira.Issue, error) {
	jql := fmt.Sprintf(
		`project = "%s" AND labels = "branch-out" AND summary ~ "%s" AND summary ~ "%s" AND status != "Closed"`,
		c.jiraConfig.ProjectKey,
		packageName,
		testName,
	)
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
		return nil, nil // No open flaky test ticket found
	}
	return &issues[0], nil // Return the first open flaky test ticket
}

// AuthType returns the type of authentication being used
func (c *Client) AuthType() string {
	if c.jiraConfig.OAuthAccessToken != "" {
		return "OAuth"
	} else if c.jiraConfig.Username != "" && c.jiraConfig.Token != "" {
		return "Basic Auth"
	}
	return "None"
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
