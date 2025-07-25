//go:build ignore

// Used to generate the config.md file.
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/smartcontractkit/branch-out/config"
)

func main() {
	var buf bytes.Buffer

	_, err := buf.WriteString("# Configuration\n\n")
	if err != nil {
		log.Fatalf("Failed to write to buffer: %v", err)
	}

	_, err = buf.WriteString("All config fields for `branch-out`.\n\nConfig fields are loaded in the following priority order:\n\n")
	if err != nil {
		log.Fatalf("Failed to write to buffer: %v", err)
	}

	_, err = buf.WriteString("1. CLI flags\n2. Environment variables\n3. `.env` file\n4. Default values\n\n")
	if err != nil {
		log.Fatalf("Failed to write to buffer: %v", err)
	}

	_, err = buf.WriteString("| Env Var | Description | Example | Flag | Short Flag | Type | Default | Required | Secret |\n")
	if err != nil {
		log.Fatalf("Failed to write to buffer: %v", err)
	}
	_, err = buf.WriteString("|---------|-------------|---------|------|------------|------|---------|----------|-------- |\n")
	if err != nil {
		log.Fatalf("Failed to write to buffer: %v", err)
	}

	fields := config.Fields

	for _, field := range fields {
		_, err = buf.WriteString(field.MarkdownTable() + "\n")
		if err != nil {
			log.Fatalf("Failed to write to buffer: %v", err)
		}
	}

	err = os.WriteFile("../config.md", buf.Bytes(), 0644)
	if err != nil {
		log.Fatalf("Failed to write to file: %v", err)
	}
	fmt.Println("Generated config.md")
}
