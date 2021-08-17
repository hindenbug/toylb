package main

import (
	"net/http/httputil"
	"net/url"
	"sync"
)

type Server struct {
	URL          *url.URL
	Alive        bool
	mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}

func (s *Server) IsAlive() bool {
	return s.Alive
}

func (s *Server) SetAlive(alive bool) {
	s.mux.Lock()
	s.Alive = alive
	s.mux.Unlock()
}
