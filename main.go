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
	"time"
)

var serverPool ServerPool

const PORT uint = 8080
const MAX_RETRIES = 3
const MAX_ATTEMPTS = 3

const (
	Attempts int = iota
	Retry
)

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

func loadBalance(w http.ResponseWriter, r *http.Request) {
	attempts := GetAttemptsFromContext(r)
	if attempts > MAX_ATTEMPTS {
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

func main() {
	var serverList string
	var port uint
	flag.StringVar(&serverList, "servers", "", "Backends attached to the load balancer, use commas to separate")
	flag.UintVar(&port, "port", PORT, "Serving port")
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

			if retries < MAX_RETRIES {
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
