package adminserver

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	"net/http"
	"os"
)

func (s *Server) announce(w http.ResponseWriter, r *http.Request) {
	logger.Infow("received announce request")
	c, err := s.e.PublishLatest(context.Background())
	if err != nil {
		msg := fmt.Sprintf("failed to announce latest metadata: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusInternalServerError, NewErrorResponse(http.StatusInternalServerError, msg))
		return
	}

	respond(w, http.StatusOK, NewOKResponse(fmt.Sprintf("announce latest metadata successfully! cid: %s", c.String()), nil))
}

func (s *Server) addFile(w http.ResponseWriter, r *http.Request) {
	logger.Infow("received import file request")

	var req ImportFileReq
	if _, err := req.ReadFrom(r.Body); err != nil {
		msg := fmt.Sprintf("failed to unmarshal request: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusBadRequest, NewErrorResponse(http.StatusBadRequest, msg))
		return
	}

	fBytes, err := os.ReadFile(req.Path)
	if err != nil {
		msg := fmt.Sprintf("failed to read file: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusInternalServerError, NewErrorResponse(http.StatusInternalServerError, msg))
		return
	}
	ctx := context.Background()
	c, err := s.e.PublishBytesData(ctx, fBytes)
	if err != nil {
		msg := fmt.Sprintf("failed to publish data: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusInternalServerError, NewErrorResponse(http.StatusInternalServerError, msg))
		return
	}

	respond(w, http.StatusOK, NewOKResponse(fmt.Sprintf("successfully add file, cid: %s", c.String()), nil))
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	logger.Infow("received sync request")

	var req SyncReq
	if _, err := req.ReadFrom(r.Body); err != nil {
		msg := fmt.Sprintf("failed to unmarshal request: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusBadRequest, NewErrorResponse(http.StatusBadRequest, msg))
		return
	}

	if err := req.Validate(); err != nil {
		msg := fmt.Sprintf("invalid sync request : %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusBadRequest, NewErrorResponse(http.StatusBadRequest, msg))
		return
	}

	_, err := s.e.Sync(context.Background(), req.Cid, req.Depth, req.StopCid)
	if err != nil {
		msg := fmt.Sprintf("failed to sync cid from Pando: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusInternalServerError, NewErrorResponse(http.StatusInternalServerError, msg))
		return
	}

	respond(w, http.StatusOK, NewOKResponse("sync successfully!", nil))
}

func (s *Server) showList(w http.ResponseWriter, r *http.Request) {
	logger.Infow("received cid list request")

	clist, err := s.e.GetPushedList(context.Background())
	if err != nil {
		msg := fmt.Sprintf("failed to get cid list: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusInternalServerError, NewErrorResponse(http.StatusInternalServerError, msg))
		return
	}

	respond(w, http.StatusOK, NewOKResponse("sync successfully!", clist))
}

func (s *Server) cat(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cidStr := vars["cid"]
	c, err := cid.Decode(cidStr)
	if err != nil {
		msg := fmt.Sprintf("invalid cid to cat: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusBadRequest, NewErrorResponse(http.StatusBadRequest, msg))
		return
	}

	res, err := s.e.CatCid(context.Background(), c)
	if err != nil {
		msg := fmt.Sprintf("failed to cat data for cid: %s: %v", c.String(), err)
		logger.Errorf(msg)
		respond(w, http.StatusInternalServerError, NewErrorResponse(http.StatusInternalServerError, msg))
		return
	}

	respond(w, http.StatusOK, NewOKResponse("cat successfully!", res))
}

func (s *Server) syncWithProvider(w http.ResponseWriter, r *http.Request) {
	logger.Infow("received provider sync request")
	var req SyncReq
	if _, err := req.ReadFrom(r.Body); err != nil {
		msg := fmt.Sprintf("failed to unmarshal request: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusBadRequest, NewErrorResponse(http.StatusBadRequest, msg))
		return
	}
	if err := req.Validate(); err != nil {
		msg := fmt.Sprintf("invalid sync request : %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusBadRequest, NewErrorResponse(http.StatusBadRequest, msg))
		return
	}

	err := s.e.SyncWithProvider(context.Background(), req.Provider, req.Depth, req.StopCid)
	if err != nil {
		msg := fmt.Sprintf("failed to sync with provider: %v", err)
		logger.Errorf(msg)
		respond(w, http.StatusInternalServerError, NewErrorResponse(http.StatusInternalServerError, msg))
		return
	}

	respond(w, http.StatusOK, NewOKResponse(fmt.Sprintf("sync with provider %s successfully", req.Provider), nil))

}

func decodePeerID(id string, w http.ResponseWriter) (peer.ID, bool) {
	peerID, err := peer.Decode(id)
	if err != nil {
		msg := "Cannot decode peer id"
		logger.Errorw(msg, "id", id, "err", err)
		respond(w, http.StatusBadRequest, NewErrorResponse(http.StatusBadRequest, msg))
		return peerID, false
	}
	return peerID, true
}

func decodeCid(id string, w http.ResponseWriter) (cid.Cid, bool) {
	c, err := cid.Decode(id)
	if err != nil {
		msg := "Cannot decode cid"
		logger.Errorw(msg, "cid", id, "err", err)
		respond(w, http.StatusBadRequest, NewErrorResponse(http.StatusBadRequest, msg))
		return c, false
	}
	return c, true
}
