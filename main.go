package main

import (
	"github.com/grumpylabs/kpf/cmd"
)

var (
	// Version information set by build flags
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
