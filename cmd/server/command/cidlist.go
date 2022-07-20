package command

import (
	"github.com/spf13/cobra"
)

func CidListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cidlist",
		Short: "display the cid list of you pushed",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := Client.R().
				SetHeader("Content-Type", "application/octet-stream").
				Get("/admin/cidlist")
			if err != nil {
				return err
			}

			return PrintResponseData(res)
		},
	}

	return cmd
}
