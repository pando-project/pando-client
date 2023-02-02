package command

import (
	"encoding/json"
	"fmt"
	adminserver "github.com/pando-project/pando-client/pkg/server/admin/http"
	"github.com/spf13/cobra"
	"time"
)

var syncReq = adminserver.SyncReq{}

func SyncCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "sync ipld nodes from Pando by cid",
		RunE: func(cmd *cobra.Command, args []string) error {
			if syncReq.Cid == "" {
				return fmt.Errorf("nil sync cid")
			}
			if err := syncReq.Validate(); err != nil {
				return err
			}
			bodyBytes, err := json.Marshal(syncReq)
			if err != nil {
				return err
			}
			// it may take long time to finish the syncing
			res, err := Client.SetTimeout(time.Hour).R().
				SetBody(bodyBytes).
				SetHeader("Content-Type", "application/octet-stream").
				Post("/admin/sync")
			defer Client.SetTimeout(10 * time.Second)
			if err != nil {
				return err
			}

			return PrintResponseData(res)
		},
	}

	cmd.Flags().StringVarP(&syncReq.Cid, "start-cid", "s", "", "head cid to sync")
	cmd.Flags().StringVarP(&syncReq.StopCid, "end-cid", "e", "", "end cid")
	cmd.Flags().IntVarP(&syncReq.Depth, "depth", "d", 0, "max depth to sync")

	return cmd
}
