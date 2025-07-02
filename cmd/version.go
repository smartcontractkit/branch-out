package cmd

import (
	"fmt"
	"runtime"
	"time"
)

// These variables are set at build time and describe the version and build of the application
var (
	version   = "dev"
	commit    = "dev"
	buildTime = time.Now().Format("2006-01-02T15:04:05.000")
	builtWith = runtime.Version()
)

// Version returns the version of the application.
func Version() string {
	return fmt.Sprintf(
		"version: %s\ncommit: %s\nbuild time: %s\nbuilt with: %s",
		version,
		commit,
		buildTime,
		builtWith,
	)
}
