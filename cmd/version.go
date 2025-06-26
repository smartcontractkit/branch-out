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

	versionString = fmt.Sprintf(
		"version: %s\ncommit: %s\nbuild time: %s\nbuilt by: %s\nbuilt with: %s",
		version,
		commit,
		buildTime,
		builtBy,
		builtWith,
	)
)
