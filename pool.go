package main

import (
	"log"
	"net/url"
	"sync/atomic"
	"time"
)

type ServerPool struct {
	servers []*Server
	current uint64
}

func (p *ServerPool) AddServer(server *Server) {
	p.servers = append(p.servers, server)
}

func (p *ServerPool) AliveServerIndex() int {
	return int(atomic.AddUint64(&p.current, uint64(1)) % uint64(len(p.servers)))
}

// get the Next alive server
func (p *ServerPool) NextServer() *Server {
	nextIndex := int(atomic.AddUint64(&p.current, uint64(1)))
	l := len(p.servers) + nextIndex

	for i := nextIndex; i < l; i++ {
		next := i % len(p.servers)
		if p.servers[next].IsAlive() {
			if i != nextIndex {
				atomic.StoreUint64(&p.current, uint64(next))
			}
			return p.servers[next]
		}
	}
	return nil
}

// SetServerStatus changes a status of a server
func (p *ServerPool) SetServerStatus(url *url.URL, alive bool) {
	for _, s := range p.servers {
		if s.URL.String() == url.String() {
			s.Alive = alive
			break
		}
	}
}

func (p *ServerPool) HealthCheck() {
	t := time.NewTicker(time.Second * 20)
	for {
		select {
		case <-t.C:
			log.Println("Starting Health Check....")

			for _, s := range p.servers {
				alive := isServerAlive(s.URL)
				s.SetAlive(alive)
				if alive {
					log.Printf("%s [%s]\n", s.URL, "UP")
				} else {
					log.Printf("%s [%s]\n", s.URL, "DOWN")
				}
			}
			log.Println("Health check done.")
		}
	}
}
