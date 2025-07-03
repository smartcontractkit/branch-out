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
	builtBy   = "local"
	builtWith = runtime.Version()
)

// Version returns the version of the application.
func Version() string {
	return fmt.Sprintf(
		"%s on commit %s, built at %s with %s by %s",
		version,
		commit,
		buildTime,
		builtWith,
		builtBy,
	)
}
