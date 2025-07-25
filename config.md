# Configuration

All config fields for `branch-out`.

Config fields are loaded in the following priority order:

1. CLI flags
2. Environment variables
3. `.env` file
4. Default values

| Env Var | Description | Example | Flag | Short Flag | Type | Default | Required | Secret |
|---------|-------------|---------|------|------------|------|---------|----------|-------- |
| LOG_LEVEL | Log level for the application | info | log-level | l | string | info | false | false |
| PORT | Port to listen on | 8080 | port |  | int | 8080 | false | false |
| LOG_PATH | Path to a log file if you want to also log to a file | /tmp/branch-out.log | log-path |  | string |  | false | false |
| GITHUB_TOKEN | GitHub personal access token, alternative to using a GitHub App. Try using (gh auth token) to generate a token. | ghp_xxxxxxxxxxxxxxxxxxxx | github-token |  | string | <nil> | false | true |
| GITHUB_BASE_URL | GitHub API base URL | https://api.github.com | github-base-url |  | string | https://api.github.com | false | false |
| GITHUB_APP_ID | GitHub App ID, alternative to using a GitHub token | 123456 | github-app-id |  | string | <nil> | false | false |
| GITHUB_PRIVATE_KEY | GitHub App private key (PEM format) | -----BEGIN RSA PRIVATE KEY-----<private-key-content>-----END RSA PRIVATE KEY----- | github-private-key |  | string |  | false | true |
| GITHUB_PRIVATE_KEY_FILE | Path to GitHub App private key file | /path/to/private-key.pem | github-private-key-file |  | string | <nil> | false | false |
| GITHUB_INSTALLATION_ID | GitHub App installation ID | 123456 | github-installation-id |  | string | <nil> | false | false |
| TRUNK_TOKEN | API token for Trunk.io | trunk_xxxxxxxxxxxxxxxxxxxx | trunk-token |  | string | <nil> | false | true |
| TRUNK_WEBHOOK_SECRET | Webhook signing secret used to verify webhooks from Trunk.io | trunk_webhook_secret | trunk-webhook-secret |  | string | <nil> | false | true |
| JIRA_BASE_DOMAIN | Jira base domain | mycompany.atlassian.net | jira-base-domain |  | string | <nil> | false | false |
| JIRA_PROJECT_KEY | Jira project key for tickets | PROJ | jira-project-key |  | string | <nil> | false | false |
| JIRA_OAUTH_CLIENT_ID | Jira OAuth client ID | jira_oauth_client_id | jira-oauth-client-id |  | string | <nil> | false | false |
| JIRA_OAUTH_CLIENT_SECRET | Jira OAuth client secret | jira_oauth_client_secret | jira-oauth-client-secret |  | string | <nil> | false | true |
| JIRA_OAUTH_ACCESS_TOKEN | Jira OAuth access token | jira_oauth_access_token | jira-oauth-access-token |  | string | <nil> | false | true |
| JIRA_OAUTH_REFRESH_TOKEN | Jira OAuth refresh token | jira_oauth_refresh_token | jira-oauth-refresh-token |  | string | <nil> | false | true |
| JIRA_USERNAME | Jira username for basic auth | user@company.com | jira-username |  | string | <nil> | false | false |
| JIRA_TOKEN | Jira API token for basic auth | jira_api_token | jira-token |  | string | <nil> | false | true |
| JIRA_TEST_FIELD_ID | If available, the ID of the custom field used to store the test name | customfield_10003 | jira-test-field-id |  | string | <nil> | false | false |
| JIRA_PACKAGE_FIELD_ID | If available, the ID of the custom field used to store the package name | customfield_10003 | jira-package-field-id |  | string | <nil> | false | false |
| JIRA_TRUNK_ID_FIELD_ID | If available, the ID of the custom field used to store the Trunk ID | customfield_10003 | jira-trunk-id-field-id |  | string | <nil> | false | false |
| AWS_REGION | AWS region for SQS | us-west-2 | aws-region |  | string | <nil> | false | false |
| AWS_SQS_QUEUE_URL | AWS SQS queue URL for webhooks payloads | https://sqs.us-west-2.amazonaws.com/123456789012/my-queue.fifo | aws-sqs-queue-url |  | string | <nil> | false | false |
