// Command qk decides what is missing or stale around the code: docs,
// contracts, threat models, test plans and operational readiness.
// It runs the same way in Claude Code hooks, pre-push and CI.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// version is set at release time with -ldflags "-X main.version=<tag>".
var version = "dev"

// schemaVersion is the quality.yaml schema version this qk reads.
const schemaVersion = 1

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "qk",
		Short:        "Quality Kit: keeps docs, contracts and readiness in step with the code",
		SilenceUsage: true,
	}
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the qk version and the manifest schema version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "qk %s (manifest schema %d)\n", version, schemaVersion)
		},
	})
	return root
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(2) // usage error; exit 1 is reserved for blocking findings
	}
}
