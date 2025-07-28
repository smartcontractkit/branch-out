package logging

import (
	"bytes"
	"io"
)

// redactWriter is a custom io.Writer that redacts sensitive information from logs.
type redactWriter struct {
	Writer  io.Writer
	Secrets []string
}

// Write implements the io.Writer interface for redactWriter.
func (rw *redactWriter) Write(p []byte) (n int, err error) {
	// Redact sensitive information from the log data
	redactedData := redactSecrets(p, rw.Secrets)
	// Write the redacted data to the underlying writer
	n, err = rw.Writer.Write(redactedData)
	if err != nil {
		return n, err
	}
	// Return the number of bytes written and no error
	return n, nil
}

// redactSecrets replaces sensitive information in the log data with "[REDACTED]".
func redactSecrets(data []byte, secrets []string) []byte {
	for _, secret := range secrets {
		data = bytes.ReplaceAll(data, []byte(secret), []byte("[REDACTED]"))
	}
	return data
}
