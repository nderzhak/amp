package main

import (
	"github.com/appcelerator/amp/cli"
	"github.com/appcelerator/amp/cli/command/version"
	"github.com/spf13/cobra"
)

// newRootCommand returns a new instance of the amp cli root command.
func newRootCommand(c cli.Interface) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "amp [OPTIONS] COMMAND [ARG...]",
		Short:             "Appcelerator Microservice Platform",
		SilenceUsage:      true,
		//SilenceErrors:     true,
		Example:           "amp version",
		PersistentPreRunE: cli.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			c.Console().Println(cmd.UsageString())
		},
	}

	cli.SetupRootCommand(cmd)

	cmd.SetOutput(c.Out())

	addCommands(cmd, c)

	return cmd
}

// addCommands adds the cli commands to the root command that we want to make available for a release.
func addCommands(cmd *cobra.Command, c cli.Interface) {
	cmd.AddCommand(
		// version
		version.NewVersionCommand(c),
	)
}