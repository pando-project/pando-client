package command

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	adminserver "pandoClient/pkg/server/admin/http"
	"time"
)

var providerSyncReq = adminserver.SyncReq{}

func ProviderSyncCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync-provider",
		Short: "sync with provider from head in Pando",
		RunE: func(cmd *cobra.Command, args []string) error {
			if providerSyncReq.Provider == "" {
				return fmt.Errorf("nil sync provider")
			}
			if err := providerSyncReq.Validate(); err != nil {
				return err
			}
			bodyBytes, err := json.Marshal(providerSyncReq)
			if err != nil {
				return err
			}
			// it may take long time to finish the syncing
			res, err := Client.SetTimeout(time.Hour).R().
				SetBody(bodyBytes).
				SetHeader("Content-Type", "application/octet-stream").
				Post("/admin/syncprovider")
			defer Client.SetTimeout(time.Second * 10)
			if err != nil {
				return err
			}

			return PrintResponseData(res)
		},
	}

	cmd.Flags().StringVarP(&providerSyncReq.StopCid, "end-cid", "e", "", "end cid")
	cmd.Flags().StringVarP(&providerSyncReq.Provider, "provider", "p", "", "provider")
	cmd.Flags().IntVarP(&providerSyncReq.Depth, "depth", "d", 0, "max depth to sync")

	return cmd
}
