package cmd

import (
	"github.com/innobead/kubefire/internal/config"
	"github.com/innobead/kubefire/pkg/script"
	"github.com/spf13/cobra"
)

var UninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall prerequisites",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := script.Download(script.UninstallPrerequisites, config.TagVersion, false); err != nil {
			return err
		}

		if err := script.Run(script.UninstallPrerequisites, config.TagVersion); err != nil {
			return err
		}

		return nil
	},
}