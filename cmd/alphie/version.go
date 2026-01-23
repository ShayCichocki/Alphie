package main

import (
	"fmt"

	"github.com/ShayCichocki/alphie/internal/version"
	"github.com/spf13/cobra"
)

// Version returns the current version
func Version() string {
	return version.Get()
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("alphie version %s\n", Version())
	},
}
