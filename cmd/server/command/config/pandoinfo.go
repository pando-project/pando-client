package config

import (
	"encoding/json"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	"net/http"
)

//{
//"code": 200,
//"message": "ok",
//
//"Data": {
//"PeerID": "12D3KooWNU48MUrPEoYh77k99RbskgftfmSm3CdkonijcM5VehS9",
//
//"APIAddresses": {
//"GRAPHQL_API": "/ip4/52.14.211.248/tcp/9012",
//"GRAPHSYNC_API": "/ip4/52.14.211.248/tcp/9013",
//"HTTP_API": "/ip4/52.14.211.248/tcp/9011"
//}
//}
//
//}

const (
	kenlbasInfoUrl = "http://localhost:9000/pando/info"
	pandoAPIUrl    = "http://localhost:9000"
)

type httpRes struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		PeerID       string `json:"peerID"`
		APIAddresses struct {
			GRAPHQL_API   string `json:"GraphQLAPI"`
			GRAPHSYNC_API string `json:"GraphSyncAPI"`
			HTTP_API      string `json:"HttpAPI"`
		} `json:"Addresses"`
	} `json:"Data"`
}

type PandoInfo struct {
	PandoMultiAddr string
	PandoPeerID    string
	PandoAPIUrl    string
	TopicName      string
}

func (pinfo *PandoInfo) AddrInfo() (*peer.AddrInfo, error) {
	multiAddress := pinfo.PandoMultiAddr + "/ipfs/" + pinfo.PandoPeerID
	peerInfo, err := peer.AddrInfoFromString(multiAddress)
	if err != nil {
		return nil, err
	}
	return peerInfo, nil
}

func GetPandoInfo() (*PandoInfo, error) {
	res, err := http.Get(kenlbasInfoUrl)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wrong http code: %v", res.StatusCode)
	}
	pinfo := new(httpRes)
	err = json.NewDecoder(res.Body).Decode(pinfo)
	if err != nil {
		return nil, err
	}
	return &PandoInfo{
		PandoMultiAddr: pinfo.Data.APIAddresses.GRAPHSYNC_API,
		PandoPeerID:    pinfo.Data.PeerID,
		PandoAPIUrl:    pandoAPIUrl,
		TopicName:      "/pando/v0.0.1",
	}, nil
}

func NewPandoInfo() PandoInfo {
	pinfo, err := GetPandoInfo()
	if err != nil {
		fmt.Printf("failed to get PandoInfo from Kenlabs http api...Please fill manually, err: %v", err.Error())
		return PandoInfo{TopicName: "/pando/v0.0.1"}
	}
	return *pinfo
}
