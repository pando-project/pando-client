package command

import (
	"github.com/ipfs/go-cid"
	"github.com/spf13/cobra"
)

var catCid string

func CatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cat",
		Short: "show IPLD Node data by cid",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := cid.Decode(catCid); err != nil {
				return err
			}
			res, err := Client.R().
				SetHeader("Content-Type", "application/octet-stream").
				Get("/admin/cat?cid=" + catCid)
			if err != nil {
				return err
			}

			return PrintResponseData(res)
		},
	}

	cmd.Flags().StringVarP(&catCid, "cid", "", "", "cid to cat")

	return cmd
}
