package command

import (
	"github.com/ipfs/go-cid"
	"github.com/spf13/cobra"
)

var catCid string

func CatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cidlist",
		Short: "display the cid list of you pushed",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := cid.Decode(catCid); err != nil {
				return err
			}
			res, err := Client.R().
				SetHeader("Content-Type", "application/octet-stream").
				Get("/admin/cidlist?cid=" + catCid)
			if err != nil {
				return err
			}

			return PrintResponseData(res)
		},
	}

	cmd.Flags().StringVarP(&catCid, "cid", "c", "", "cid to cat")

	return cmd
}
