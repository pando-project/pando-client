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
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"pando-client/cmd/server/command/config"
	"pando-client/pkg/engine"
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

/**
	1. create libp2p host and store
	2. get(set) PandoInfo
	3. keep connectness withPando
	4. create Engine and Start
	5. sync data you want(with Pando). In the example, we sync with dealbot provider from Pando.
	(6.) deal with received data. (compute the success rate from received FinishedTask)
**/
func main() {
	// 1. create libp2p host and store
	peerID, privKey, err := getPeerIDFromPrivateKeyStr(clientPrivateStr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("client peerID: %v\n", peerID.String())

	h, err := libp2p.New(
		// Use the keypair generated during init
		libp2p.Identity(privKey),
	)
	ds, err := leveldb.NewDatastore("", nil)
	if err != nil {
		panic(err)
	}
	bs := blockstore.NewBlockstore(ds)
	// 2. get(set) PandoInfo
	pandoInfo, err := config.GetPandoInfo()
	if err != nil {
		panic(err)
	}
	pandoAddrInfo, err := pandoInfo.AddrInfo()
	if err != nil {
		panic(err)
	}
	// 3. keep connectness withPando
	err = h.Connect(context.Background(), *pandoAddrInfo)
	if err != nil {
		panic(err)
	}

	// 4. create Engine(init)
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

	// (6.) deal with received data
	ch := make(chan Status, 0)
	// compute for success rate got from linksystem
	go func() {
		var total float32
		var success float32
		for status := range ch {
			total += 1
			if status == Status(4) {
				success += 1
			}
			successRate := success / total
			fmt.Printf("total job numbers: %d\n", int(total))
			fmt.Println("success rate: ", successRate)
		}
	}()
	lsys := MkLinkSystem(bs, ch)
	//  end (6.)

	// 4. create Engine and Start
	e, err := engine.New(
		engine.WithCheckInterval(config.Duration(time.Second*5)),
		engine.WithPandoAPIClient(pandoAPIUrl, time.Second*10),
		engine.WithPandoAddrinfo(*pandoAddrInfo),
		engine.WithDatastore(ds),
		engine.WithDataTransfer(dt),
		engine.WithHost(h),
		engine.WithTopicName(pandoInfo.TopicName),
		engine.WithPublisherKind("dtsync"),
		engine.WithLinkSystem(&lsys),
	)
	if err != nil {
		panic(err)
	}
	err = e.Start(context.Background())
	if err != nil {
		panic(err)
	}
	// 5. sync data you want(with Pando). In the example, we sync with dealbot provider from Pando.
	err = e.SyncWithProvider(context.Background(), dealbotIDStr, 1000000, "")
	if err != nil {
		panic(err)
	}
	err = e.Shutdown()
	if err != nil {
		panic(err)
	}
}
