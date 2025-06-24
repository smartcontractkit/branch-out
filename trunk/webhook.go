package trunk

import (
	"fmt"

	"github.com/rs/zerolog"
)

// ReceiveWebhook processes a Trunk webhook and returns an error if the webhook is invalid.
func ReceiveWebhook(l zerolog.Logger, payload []byte) error {
	l.Info().Msg("Received trunk webhook")
	// First, quickly determine the webhook type without full parsing
	webhookType, err := GetWebhookType(payload)
	if err != nil {
		l.Error().Err(err).Str("payload", string(payload)).Msg("Failed to determine webhook type")
		return err
	}

	l = l.With().Str("webhook_type", string(webhookType)).Logger()

	// Parse the full event
	event, err := ParseWebhookEvent(payload)
	if err != nil {
		l.Error().Err(err).Msg("Failed to parse webhook event")
		return err
	}

	// Process different event types
	switch webhookType {
	case WebhookTypeQuarantiningSettingChanged:
		return handleQuarantiningSettingChanged(l, event)

	case WebhookTypeStatusChanged:
		return handleStatusChanged(l, event)
	}
	return fmt.Errorf("unknown webhook type '%s'", webhookType)
}

// handleQuarantiningSettingChanged processes quarantining setting change events
func handleQuarantiningSettingChanged(l zerolog.Logger, event WebhookEvent) error {
	l.Info().Msg("Processing quarantining setting changed event")

	testCase := event.GetTestCase()
	l.Info().
		Str("test_id", testCase.ID).
		Str("test_name", testCase.Name).
		Str("file_path", testCase.FilePath).
		Bool("quarantined", testCase.Quarantined).
		Msg("Test case quarantine setting changed")

	// TODO: Implement quarantine logic
	// - Parse Go test files
	// - Add/remove t.Skip() calls
	// - Create PR with changes

	return nil
}

// handleStatusChanged processes status change events
func handleStatusChanged(l zerolog.Logger, event WebhookEvent) error {
	l.Info().Msg("Processing status changed event")

	testCase := event.GetTestCase()
	l.Info().
		Str("test_id", testCase.ID).
		Str("test_name", testCase.Name).
		Str("file_path", testCase.FilePath).
		Str("status", testCase.Status.Value).
		Msg("Test case status changed")

	// TODO: Implement status change logic
	// - Maybe create issues for consistently failing tests
	// - Update monitoring/alerting

	return nil
}
