package main

import (
	"context"
	"encoding/base64"
	"fmt"
	datatransfer "github.com/filecoin-project/go-data-transfer/impl"
	dtnetwork "github.com/filecoin-project/go-data-transfer/network"
	gstransport "github.com/filecoin-project/go-data-transfer/transport/graphsync"
	leveldb "github.com/ipfs/go-ds-leveldb"
	gsimpl "github.com/ipfs/go-graphsync/impl"
	gsnet "github.com/ipfs/go-graphsync/network"
	logging "github.com/ipfs/go-log/v2"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"pandoClient/cmd/server/command/config"
	"pandoClient/pkg/engine"
	"time"
)

const (
	pandoAPIUrl      = "http://52.14.211.248:9011"
	clientPrivateStr = "CAESQAycIStrQXBoxgf2pEazDLoZbL8WCLX5GIb69dl4x2mJMpukCAPbzq1URPtKen4Bpxfz9et2exWhfAfZ/RG30ts="
	dealbotIDStr     = "12D3KooWNnK4gnNKmh6JUzRb34RqNcBahN5B8v18DsMxQ8mCqw81"
)

func getPeerIDFromPrivateKeyStr(privateKeyStr string) (peer.ID, crypto.PrivKey, error) {
	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKeyStr)
	if err != nil {
		return "", nil, err
	}

	privateKey, err := crypto.UnmarshalPrivateKey(privateKeyBytes)
	if err != nil {
		return "", nil, err
	}

	peerID, err := peer.IDFromPrivateKey(privateKey)
	if err != nil {
		return "", nil, err
	}

	return peerID, privateKey, nil
}

func main() {
	_ = logging.SetLogLevel("*", "info")

	peerID, privKey, err := getPeerIDFromPrivateKeyStr(clientPrivateStr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("client peerID: %v\n", peerID.String())

	h, err := libp2p.New(
		// Use the keypair generated during init
		libp2p.Identity(privKey),
	)

	pandoInfo, err := config.GetPandoInfo()
	if err != nil {
		panic(err)
	}
	pandoAddrInfo, err := pandoInfo.AddrInfo()
	if err != nil {
		panic(err)
	}

	ds, err := leveldb.NewDatastore("", nil)
	if err != nil {
		panic(err)
	}

	gsnet := gsnet.NewFromLibp2pHost(h)
	dtNet := dtnetwork.NewFromLibp2pHost(h)
	gs := gsimpl.New(context.Background(), gsnet, cidlink.DefaultLinkSystem())
	tp := gstransport.NewTransport(h.ID(), gs)
	dt, err := datatransfer.NewDataTransfer(ds, dtNet, tp)
	if err != nil {
		panic(err)
	}
	err = dt.Start(context.Background())
	if err != nil {
		panic(err)
	}

	err = h.Connect(context.Background(), *pandoAddrInfo)
	if err != nil {
		panic(err)
	}

	e, err := engine.New(
		engine.WithCheckInterval(config.Duration(time.Second*5)),
		engine.WithPandoAPIClient(pandoAPIUrl, time.Second*10),
		engine.WithPandoAddrinfo(*pandoAddrInfo),
		engine.WithDatastore(ds),
		engine.WithDataTransfer(dt),
		engine.WithHost(h),
		engine.WithTopicName(pandoInfo.TopicName),
		engine.WithPublisherKind("dtsync"),
	)
	if err != nil {
		panic(err)
	}
	err = e.Start(context.Background())
	if err != nil {
		panic(err)
	}
	err = e.SyncWithProvider(context.Background(), dealbotIDStr, 3, "")
	if err != nil {
		panic(err)
	}
	err = e.Shutdown()
	if err != nil {
		panic(err)
	}
}
