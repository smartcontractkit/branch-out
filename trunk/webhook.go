package trunk

import (
	"fmt"

	"github.com/rs/zerolog"
)

// ReceiveWebhook processes a Trunk webhook and returns an error if the webhook is invalid.
func ReceiveWebhook(l zerolog.Logger, payload []byte) error {
	l.Info().Msg("Received trunk webhook")

	// Figure out what type of event this is
	webhookType, err := GetWebhookType(payload)
	if err != nil {
		l.Error().Err(err).Str("payload", string(payload)).Msg("Failed to determine webhook type")
		return err
	}

	l = l.With().Str("webhook_type", string(webhookType)).Logger()

	event, err := ParseWebhookEvent(payload)
	if err != nil {
		l.Error().Err(err).Msg("Failed to parse webhook event")
		return err
	}

	// Handle different event types
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

	// TODO: Decide what to do with this

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
		Str("reason", testCase.Status.Reason).
		Msg("Test case status changed")

	// TODO: Decide what to do with this
	l.Debug().Msg("Status change processed (no action taken)")

	return nil
}
