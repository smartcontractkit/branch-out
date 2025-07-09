// Package main is the entry point for the branch-out application.
package main

//go:generate go tool mockery

import (
	"github.com/smartcontractkit/branch-out/cmd"
)

func main() {
	cmd.Execute()
}
