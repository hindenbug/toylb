package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

type Backend struct {
	URL          *url.URL
	Alive        bool
	ReverseProxy *httputil.ReverseProxy
}

type ServerPool struct {
	backends []*Backend
	current  uint64
}

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

	for _, token := range serverTokens {
		serverUrl, err := url.Parse(token)

		if err != nil {
			log.Fatal(err)
		}

		reverseProxy := httputil.NewSingleHostReverseProxy(serverUrl)

		server := http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: http.HandlerFunc(reverseProxy.ServeHTTP),
		}

		log.Printf("Load Balancer started at :%d\n", port)
		if err := server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}
}
