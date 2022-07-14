package command

import (
	"github.com/spf13/cobra"
	"pandoClient/cmd/server/command/config"
)

func InitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize client config file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.Info("Initializing provider config file")

			// Check that the config root exists and it writable.
			configRoot, err := config.PathRoot()
			if err != nil {
				return err
			}

			if err = checkWritable(configRoot); err != nil {
				return err
			}

			configFile, err := config.Filename(configRoot)
			if err != nil {
				return err
			}

			if fileExists(configFile) {
				return config.ErrInitialized
			}

			cfg, err := config.Init(cmd.OutOrStdout())
			if err != nil {
				return err
			}

			// Use values from flags to override defaults
			// cfg.Identity = struct{}{}

			return cfg.Save(configFile)
		},
	}
}
