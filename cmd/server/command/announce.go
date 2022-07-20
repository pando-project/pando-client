package command

import (
	"github.com/spf13/cobra"
)

func AnnounceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "announce",
		Short: "announce latest metadata to pubusb",
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := Client.R().
				SetHeader("Content-Type", "application/octet-stream").
				Post("/admin/announce")
			if err != nil {
				return err
			}

			return PrintResponseData(res)
		},
	}

	return cmd
}
