package adminserver

import (
	"context"
	"net"
	"net/http"
	"pando-client/pkg/engine"
	"pando-client/pkg/util/log"

	"github.com/gorilla/mux"
	"github.com/libp2p/go-libp2p-core/host"
)

var logger = log.NewSubsystemLogger()

type Server struct {
	server *http.Server
	l      net.Listener
	h      host.Host
	e      *engine.Engine
}

func New(h host.Host, e *engine.Engine, o ...Option) (*Server, error) {
	opts, err := newOptions(o...)
	if err != nil {
		return nil, err
	}

	l, err := net.Listen("tcp", opts.listenAddr)
	if err != nil {
		return nil, err
	}

	r := mux.NewRouter().StrictSlash(true)
	server := &http.Server{
		Handler:      r,
		ReadTimeout:  opts.readTimeout,
		WriteTimeout: opts.writeTimeout,
	}
	s := &Server{server, l, h, e}

	// Set protocol handlers
	r.HandleFunc("/admin/announce", s.announce).
		Methods(http.MethodPost)

	r.HandleFunc("/admin/addfile", s.addFile).
		Methods(http.MethodPost)

	r.HandleFunc("/admin/sync", s.sync).
		Methods(http.MethodPost)

	r.HandleFunc("/admin/cidlist", s.showList).
		Methods(http.MethodGet)

	r.HandleFunc("/admin/cat/{cid}", s.cat).
		Methods(http.MethodGet)

	r.HandleFunc("/admin/syncprovider", s.syncWithProvider).
		Methods(http.MethodPost)

	return s, nil
}

func (s *Server) Start() error {
	logger.Infow("admin http server listening", "addr", s.l.Addr())
	return s.server.Serve(s.l)
}

func (s *Server) Shutdown(ctx context.Context) error {
	logger.Info("admin http server shutdown")
	return s.server.Shutdown(ctx)
}
