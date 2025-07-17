package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/rs/zerolog"
	"github.com/smartcontractkit/branch-out/trunk"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Get payload path from command line argument
	if len(os.Args) < 2 {
		logger.Fatal().Msg("Usage: go run main.go <payload-file-path>")
	}
	payloadPath := os.Args[1]

	// Read the test payload
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		logger.Fatal().Err(err).Str("payload_path", payloadPath).Msg("Failed to read test payload")
	}

	// Get the webhook secret from environment
	webhookSecret := os.Getenv("TRUNK_WEBHOOK_SECRET")
	if webhookSecret == "" {
		logger.Fatal().Msg("TRUNK_WEBHOOK_SECRET environment variable is required")
	}

	// Get the webhook URL from environment
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = "http://localhost:8181/webhooks/trunk" // Default for local testing set to TailScale if needed.
	}

	// Create a request
	req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(payload))
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")

	// Self-sign the request
	signedReq, err := trunk.SelfSignWebhookRequest(logger, req, webhookSecret)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to self-sign request")
	}

	// Send the signed request
	client := &http.Client{}
	resp, err := client.Do(signedReq)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to send request")
	}
	defer resp.Body.Close()

	// Read and print the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to read response")
	}

	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Response: %s\n", string(body))
}
