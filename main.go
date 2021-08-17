package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Attempts int = iota
	Retry
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

func (s *Server) SetAlive(alive bool) {
	s.mux.Lock()
	s.Alive = alive
	s.mux.Unlock()
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

func loadBalance(w http.ResponseWriter, r *http.Request) {
	attempts := GetAttemptsFromContext(r)
	if attempts > 3 {
		log.Printf("%s(%s) Max attempts reached, terminating\n", r.RemoteAddr, r.URL.Path)
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	server := serverPool.NextServer()
	if server != nil {
		server.ReverseProxy.ServeHTTP(w, r)
		return
	}

	http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
}

func GetRetriesFromContext(r *http.Request) int {
	if retry, ok := r.Context().Value(Retry).(int); ok {
		return retry
	}

	return 0
}

func GetAttemptsFromContext(r *http.Request) int {
	if attempts, ok := r.Context().Value(Attempts).(int); ok {
		return attempts
	}

	return 1
}

func isServerAlive(u *url.URL) bool {
	timeout := 1 * time.Second

	conn, err := net.DialTimeout("tcp", u.Host, timeout)

	if err != nil {
		log.Println("Site unreachable, error: ", err)
		return false
	}

	defer conn.Close()

	return true
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

		reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
			log.Printf("[%s] %s\n", serverUrl.Host, e.Error())
			retries := GetRetriesFromContext(r)

			if retries < 3 {
				select {
				case <-time.After(10 * time.Millisecond):
					ctx := context.WithValue(r.Context(), Retry, retries+1)
					reverseProxy.ServeHTTP(w, r.WithContext(ctx))
				}
				return
			}

			// after 3 retries, set server status as down
			serverPool.SetServerStatus(serverUrl, false)

			attempts := GetAttemptsFromContext(r)
			log.Printf("%s(%s) Attempting retry %d\n", r.RemoteAddr, r.URL.Path, attempts)
			ctx := context.WithValue(r.Context(), Attempts, attempts+1)
			loadBalance(w, r.WithContext(ctx))
		}

		// add server to ServerPool
		serverPool.AddServer(&Server{URL: serverUrl, Alive: true, ReverseProxy: reverseProxy})
		log.Printf("Configured instance: %s\n", serverUrl)
	}

	// create http server
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(loadBalance),
	}

	// start health checks
	go serverPool.HealthCheck()

	log.Printf("Load Balancer started at :%d\n", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

}
