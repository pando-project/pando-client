package command

import (
	"github.com/spf13/cobra"
)

var ExampleUsage = `
# Init PandoClient configs(default path is ~/.pandoclient/config).
pando-server init

# StartHttpServer pando server.
pando-server daemon
`

func NewRoot() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:        "pando-server",
		Short:      "Pando server cli",
		Example:    ExampleUsage,
		SuggestFor: []string{"pando-server"},
	}

	rootCmd.PersistentFlags().StringVarP(&PClientBaseURL, "pclient", "c", "http://127.0.0.1:9022",
		"set pando client url")
	NewClient(PClientBaseURL)

	childCommands := []*cobra.Command{
		InitCmd(),
		DaemonCmd(),
		AnnounceCommand(),
		AddFileCommand(),
		SyncCommand(),
		ProviderSyncCommand(),
		CidListCommand(),
		CatCommand(),
	}
	rootCmd.AddCommand(childCommands...)

	return rootCmd
}
