package command

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	adminserver "pando-client/pkg/server/admin/http"
)

var req = adminserver.ImportFileReq{}

func AddFileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "add file and update to Pando",
		RunE: func(cmd *cobra.Command, args []string) error {
			if req.Path == "" {
				return fmt.Errorf("nil path")
			}
			bodyBytes, err := json.Marshal(req)
			if err != nil {
				return err
			}
			res, err := Client.R().
				SetBody(bodyBytes).
				SetHeader("Content-Type", "application/octet-stream").
				Post("/admin/addfile")
			if err != nil {
				return err
			}

			return PrintResponseData(res)
		},
	}

	cmd.Flags().StringVarP(&req.Path, "path", "p", "", "file to add, required")

	return cmd
}
