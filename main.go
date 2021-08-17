package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
)

type Server struct {
	URL          *url.URL
	Alive        bool
	mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}

type ServerPool struct {
	servers []*Server
	current uint64
}

func (s *Server) IsAlive() bool {
	return s.Alive
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

func loadBalance(w http.ResponseWriter, r *http.Request) {
	server := serverPool.NextServer()

	if server != nil {
		server.ReverseProxy.ServeHTTP(w, r)
		return
	}

	http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
}

var serverPool ServerPool

func main() {
	var serverList string
	var port int
	flag.StringVar(&serverList, "servers", "", "Backends attached to the load balancer, use commas to separate")
	flag.IntVar(&port, "port", 8080, "Serving port")
	flag.Parse()

	if len(serverList) == 0 {
		log.Fatal("At least one instance needed for the LB")
		panic(-1)
	}

	serverTokens := strings.Split(serverList, ",")

	// parse servers
	for _, token := range serverTokens {
		serverUrl, err := url.Parse(token)

		if err != nil {
			log.Fatal(err)
		}

		// initialize reverse proxy
		reverseProxy := httputil.NewSingleHostReverseProxy(serverUrl)

		// add server to ServerPool
		serverPool.AddServer(&Server{URL: serverUrl, Alive: true, ReverseProxy: reverseProxy})
		log.Printf("Configured instance: %s\n", serverUrl)
	}

	// create http server
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(loadBalance),
	}

	log.Printf("Load Balancer started at :%d\n", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

}
