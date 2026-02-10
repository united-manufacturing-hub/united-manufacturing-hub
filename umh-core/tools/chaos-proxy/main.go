// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"log"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

func main() {
	listenAddr := flag.String("listen", ":8090", "address to listen on")
	targetURL := flag.String("target", "https://management.umh.app", "target URL to proxy to")
	dropEvery := flag.Int("drop-every", 3, "drop every n-th connection (0 = disable)")
	inspectDelay := flag.Bool("inspect-delay", false, "simulated packet inspection delay (e.g., 100ms)")
	longPoll := flag.Bool("long-poll", false, "enable long polling simulation")
	longPollMu := flag.Float64("long-poll-mu", 8.5, "lognormal mu (ln of median delay in ms, e.g. 8.5 ≈ 5s median)")
	longPollSigma := flag.Float64("long-poll-sigma", 1.2, "lognormal sigma (spread, higher = more variance)")
	longPollCap := flag.Int("long-poll-cap", 31000, "max long poll delay in ms (cap for outliers)")
	longPollKillPct := flag.Int("long-poll-kill-pct", 20, "percentage chance to kill connection during long poll (0-100)")
	longPollMethod := flag.String("long-poll-method", "", "only apply long-poll delay to this HTTP method (e.g. GET). Empty = all methods")
	longPollPath := flag.String("long-poll-path", "", "only apply long-poll to requests whose path contains this substring (e.g. /v2/instance/pull)")
	flag.Parse()

	target, err := url.Parse(*targetURL)
	if err != nil {
		log.Fatalf("invalid target URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the Director to set the Host header to the target host
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)
		r.Host = target.Host
	}

	var requestCount uint64

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddUint64(&requestCount, 1)

		if *dropEvery > 0 && count%uint64(*dropEvery) == 0 {
			log.Printf("sending EOF to interrupt connection %d", count)
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					if tcpConn, ok := conn.(*net.TCPConn); ok {
						tcpConn.CloseWrite()
					} else {
						conn.Close()
					}
					return
				}
			}
			// Fallback: just close without response
			return
		}

		log.Printf("proxying request %d: %s %s", count, r.Method, r.URL.Path)

		// Simulate long polling by holding the connection before proxying
		methodMatch := *longPollMethod == "" || strings.EqualFold(r.Method, *longPollMethod)
		pathMatch := *longPollPath == "" || strings.Contains(r.URL.Path, *longPollPath)
		if *longPoll && methodMatch && pathMatch {
			// Lognormal distribution: most delays cluster around the median, with occasional long tails
			sample := math.Exp(*longPollMu + *longPollSigma*rand.NormFloat64())
			delay := int(math.Min(sample, float64(*longPollCap)))
			if delay < 1 {
				delay = 1
			}
			log.Printf("long polling request %d, holding for %d ms...", count, delay)

			// Randomly schedule a mid-wait kill in a goroutine
			if *longPollKillPct > 0 && rand.IntN(100) < *longPollKillPct {
				killAfter := rand.IntN(delay)
				log.Printf("will kill connection %d after %d ms (mid long poll)", count, killAfter)
				hj, ok := w.(http.Hijacker)
				if !ok {
					return
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					return
				}
				go func() {
					time.Sleep(time.Duration(killAfter) * time.Millisecond)
					log.Printf("killing connection %d during long poll", count)
					if tcpConn, ok := conn.(*net.TCPConn); ok {
						tcpConn.CloseWrite()
					} else {
						conn.Close()
					}
				}()
				// Block the handler for the full delay so the goroutine can kill mid-wait
				time.Sleep(time.Duration(delay) * time.Millisecond)
				return
			}

			time.Sleep(time.Duration(delay) * time.Millisecond)
			log.Printf("long poll complete for request %d, proxying now", count)
		}

		// Simulate packet inspection in a goroutine (async, no blocking)
		if *inspectDelay {
			go func(reqNum uint64) {
				min := uint(0)
				max := uint(31000)
				thisDelay := rand.UintN(max-min+1) + min
				log.Printf("inspecting request %d...", reqNum)
				log.Printf("delaying with %d ms...", thisDelay)
				time.Sleep(time.Duration(thisDelay) * time.Millisecond)
				log.Printf("inspection complete for request %d", reqNum)
			}(count)
		}

		proxy.ServeHTTP(w, r)
	})

	log.Printf("reverse proxy listening on %s, forwarding to %s", *listenAddr, *targetURL)
	if *dropEvery > 0 {
		log.Printf("sending EOF every %d-th connection", *dropEvery)
	}
	if *longPoll {
		median := math.Exp(*longPollMu)
		methodFilter := "all methods"
		if *longPollMethod != "" {
			methodFilter = *longPollMethod + " only"
		}
		pathFilter := "all paths"
		if *longPollPath != "" {
			pathFilter = "path contains " + *longPollPath
		}
		log.Printf("long polling enabled (lognormal mu=%.1f sigma=%.1f, median=%.0f ms, cap=%d ms, %d%% kill chance, %s, %s)",
			*longPollMu, *longPollSigma, median, *longPollCap, *longPollKillPct, methodFilter, pathFilter)
	}

	if err := http.ListenAndServe(*listenAddr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
