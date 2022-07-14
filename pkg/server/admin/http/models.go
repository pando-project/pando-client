package adminserver

import (
	"encoding/json"
	"fmt"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"net/http"
)

type (
	ImportFileReq struct {
		// The path to the added file
		Path string `json:"path"`
	}
	ImportFileRes struct {
		// The lookup Key associated to the imported CAR.
		Path string `json:"path"`
		// The CID of the advertisement generated as a result of import.
		MetaId cid.Cid `json:"meta_id"`
	}

	SyncReq struct {
		Cid      string `json:"cid"`
		Provider string `json:"provider"`
		Depth    int    `json:"depth"`
		StopCid  string `json:"stop_cid"`
	}

	ResponseJson struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"Data"`
	}
)

func NewErrorResponse(code int, message string) *ResponseJson {
	return &ResponseJson{
		Code:    code,
		Message: message,
	}
}

func NewOKResponse(message string, data interface{}) *ResponseJson {
	var res interface{}
	byteData, ok := data.([]byte)
	if ok {
		err := json.Unmarshal(byteData, &res)
		if err != nil {
			// todo failed unmarshal the []byte result(json)
		} else {
			return &ResponseJson{
				Code:    http.StatusOK,
				Message: message,
				Data:    res,
			}
		}
	}
	return &ResponseJson{
		Code:    http.StatusOK,
		Message: message,
		Data:    data,
	}
}

func (sq *SyncReq) Validate() error {
	if sq.Cid != "" {
		_, err := cid.Decode(sq.Cid)
		if err != nil {
			return err
		}
	}
	if sq.Depth != 0 {
		if sq.Depth < 0 {
			return fmt.Errorf("depth must be positive")
		}
	}
	if sq.Provider != "" {
		_, err := peer.Decode(sq.Provider)
		if err != nil {
			return err
		}
	}
	if sq.StopCid != "" {
		_, err := cid.Decode(sq.Cid)
		if err != nil {
			return err
		}
	}
	return nil
}
