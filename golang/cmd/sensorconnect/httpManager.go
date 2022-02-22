package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

var cP sync.Map

func GetHTTPClient(url string) (client http.Client) {
	rawClient, _ := cP.LoadOrStore(url, http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          100,
			MaxConnsPerHost:       0,
			IdleConnTimeout:       10 * time.Second,
			TLSHandshakeTimeout:   1 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	})

	client = rawClient.(http.Client)
	return
}
