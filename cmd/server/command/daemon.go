package command

import (
	"context"
	"errors"
	"fmt"
	datatransfer "github.com/filecoin-project/go-data-transfer/impl"
	dtnetwork "github.com/filecoin-project/go-data-transfer/network"
	gstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	leveldb "github.com/ipfs/go-ds-leveldb"
	gsimpl "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"
	"github.com/ipfs/go-ipfs/core/bootstrap"
	logging "github.com/ipfs/go-log/v2"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"os"
	"pandoClient/cmd/server/command/config"
	"pandoClient/pkg/engine"
	adminserver "pandoClient/pkg/server/admin/http"
	"pandoClient/pkg/util/log"
	"time"
)

var logger = log.NewSubsystemLogger()

var (
	ErrDaemonStart = errors.New("daemon did not start correctly")
	ErrDaemonStop  = errors.New("daemon did not stop gracefully")
)

const (
	// shutdownTimeout is the duration that a graceful shutdown has to complete
	shutdownTimeout = 5 * time.Second
)

func DaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "start PandoClient server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				if err == config.ErrNotInitialized {
					return errors.New("PandoClient is not initialized\nTo initialize, run using the \"init\" command")
				}
				return fmt.Errorf("cannot load config file: %w", err)
			}

			_ = logging.SetLogLevel("*", "debug")

			ctx, cancelp2p := context.WithCancel(cmd.Context())
			defer cancelp2p()

			peerID, privKey, err := cfg.Identity.Decode()
			if err != nil {
				return err
			}

			p2pmaddr, err := multiaddr.NewMultiaddr(cfg.P2pServer.ListenMultiaddr)
			if err != nil {
				return fmt.Errorf("bad p2p address in config %s: %s", cfg.P2pServer.ListenMultiaddr, err)
			}
			h, err := libp2p.New(
				// Use the keypair generated during init
				libp2p.Identity(privKey),
				//Listen to p2p addr specified in config
				libp2p.ListenAddrs(p2pmaddr),
			)
			if err != nil {
				return err
			}
			logger.Infow("libp2p host initialized", "host_id", h.ID(), "multiaddr", p2pmaddr)

			// Initialize datastore
			if cfg.Datastore.Type != "levelds" {
				return fmt.Errorf("only levelds datastore type supported, %q not supported", cfg.Datastore.Type)
			}
			dataStorePath, err := config.Path("", cfg.Datastore.Dir)
			if err != nil {
				return err
			}
			err = checkWritable(dataStorePath)
			if err != nil {
				return err
			}
			ds, err := leveldb.NewDatastore(dataStorePath, nil)
			if err != nil {
				return err
			}

			gsnet := gsnet.NewFromLibp2pHost(h)
			dtNet := dtnetwork.NewFromLibp2pHost(h)
			gs := gsimpl.New(context.Background(), gsnet, cidlink.DefaultLinkSystem())
			tp := gstransport.NewTransport(h.ID(), gs)
			dt, err := datatransfer.NewDataTransfer(ds, dtNet, tp)
			if err != nil {
				return err
			}
			err = dt.Start(context.Background())
			if err != nil {
				return err
			}

			pandoAddrInfo, err := cfg.PandoInfo.AddrInfo()
			if err != nil {
				logger.Errorf("wrong pando addr, %s", pandoAddrInfo.String())
			}

			eng, err := engine.New(
				engine.WithPersistAfterSend(cfg.IngestCfg.PersistAfterSend),
				engine.WithMaxIntervalToRepublish(cfg.IngestCfg.MaxIntervalToRepublish),
				engine.WithCheckInterval(cfg.IngestCfg.CheckInterval),
				engine.WithPandoAPIClient(cfg.PandoInfo.PandoAPIUrl, time.Second*10),
				engine.WithPandoAddrinfo(*pandoAddrInfo),
				engine.WithDatastore(ds),
				engine.WithDataTransfer(dt),
				engine.WithHost(h),
				engine.WithTopicName(cfg.PandoInfo.TopicName),
				engine.WithPublisherKind(engine.PublisherKind(cfg.IngestCfg.PublisherKind)),
			)
			if err != nil {
				return err
			}

			err = eng.Start(ctx)
			if err != nil {
				return err
			}

			addr, err := cfg.AdminServer.ListenNetAddr()
			if err != nil {
				return err
			}
			adminServer, err := adminserver.New(
				h,
				eng,
				adminserver.WithListenAddr(addr),
				adminserver.WithReadTimeout(time.Duration(cfg.AdminServer.ReadTimeout)),
				adminserver.WithWriteTimeout(time.Duration(cfg.AdminServer.WriteTimeout)),
			)

			if err != nil {
				return err
			}
			logger.Infow("admin server initialized", "address", cfg.AdminServer.ListenMultiaddr)

			errChan := make(chan error, 1)
			fmt.Fprintf(cmd.ErrOrStderr(), "Starting admin server on %s ...", cfg.AdminServer.ListenMultiaddr)
			go func() {
				errChan <- adminServer.Start()
			}()

			// If there are bootstrap peers and bootstrapping is enabled, then try to
			// connect to the minimum set of peers.
			if cfg.Bootstrap.MinimumPeers != 0 {
				addrs, err := cfg.Bootstrap.PeerAddrs()
				if err != nil {
					return fmt.Errorf("bad bootstrap peer: %s", err)
				}

				pandoAddrInfo, err := cfg.PandoInfo.AddrInfo()
				if err != nil {
					return fmt.Errorf("invalid pando addrinfo: %s", err)
				}
				logger.Infow(pandoAddrInfo.String())

				addrs = append(addrs, *pandoAddrInfo)

				bootCfg := bootstrap.BootstrapConfigWithPeers(addrs)
				bootCfg.MinPeerThreshold = cfg.Bootstrap.MinimumPeers
				// move to config after
				bootCfg.Period = time.Second * 30
				bootCfg.ConnectionTimeout = time.Second * 5

				bootstrapper, err := bootstrap.Bootstrap(peerID, h, nil, bootCfg)
				if err != nil {
					return fmt.Errorf("bootstrap failed: %s", err)
				}
				defer bootstrapper.Close()
			}

			var finalErr error
			// Keep process running.
			select {
			case <-cmd.Context().Done():
			case err = <-errChan:
				logger.Errorw("Failed to start server", "err", err)
				finalErr = ErrDaemonStart
			}

			logger.Infow("Shutting down daemon")

			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			go func() {
				// Wait for context to be canceled. If timeout, then exit with error.
				<-shutdownCtx.Done()
				if shutdownCtx.Err() == context.DeadlineExceeded {
					fmt.Println("Timed out on shutdown, terminating...")
					os.Exit(-1)
				}
			}()

			if err = eng.Shutdown(); err != nil {
				logger.Errorf("Error closing provider core: %s", err)
				finalErr = ErrDaemonStop
			}

			if err = ds.Close(); err != nil {
				logger.Errorf("Error closing provider datastore: %s", err)
				finalErr = ErrDaemonStop
			}

			// cancel libp2p server
			cancelp2p()

			if err = adminServer.Shutdown(shutdownCtx); err != nil {
				logger.Errorw("Error shutting down admin server: %s", err)
				finalErr = ErrDaemonStop
			}
			logger.Infow("node stopped")
			return finalErr
		},
	}
}
