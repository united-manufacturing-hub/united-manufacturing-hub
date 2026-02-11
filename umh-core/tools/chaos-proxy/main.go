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
	"context"
	"errors"
	"flag"
	"log"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	listenAddr := flag.String("listen", ":8090", "address to listen on")
	targetURL := flag.String("target", "https://management.umh.app", "target URL to proxy to")
	dropEvery := flag.Int("drop-every", 3, "drop every n-th connection (0 = disable)")
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

	if (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		log.Fatalf("--target must be a valid http/https URL, got %q", *targetURL)
	}
	if *dropEvery < 0 {
		log.Fatalf("--drop-every must be >= 0, got %d", *dropEvery)
	}
	if *longPollKillPct < 0 || *longPollKillPct > 100 {
		log.Fatalf("--long-poll-kill-pct must be 0-100, got %d", *longPollKillPct)
	}
	if *longPollCap <= 0 {
		log.Fatalf("--long-poll-cap must be > 0, got %d", *longPollCap)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

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
			if !ok {
				log.Printf("hijack not supported for request %d, dropping without response", count)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				log.Printf("hijack failed for request %d: %v, dropping without response", count, err)
				return
			}
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				tcpConn.CloseWrite()
			} else {
				conn.Close()
			}
			return
		}

		log.Printf("proxying request %d: %s %s", count, r.Method, r.URL.Path)

		methodMatch := *longPollMethod == "" || strings.EqualFold(r.Method, *longPollMethod)
		pathMatch := *longPollPath == "" || strings.Contains(r.URL.Path, *longPollPath)
		if *longPoll && methodMatch && pathMatch {
			sample := math.Exp(*longPollMu + *longPollSigma*rand.NormFloat64())
			delay := int(math.Min(sample, float64(*longPollCap)))
			if delay < 1 {
				delay = 1
			}
			log.Printf("long polling request %d, holding for %d ms...", count, delay)

			if *longPollKillPct > 0 && rand.IntN(100) < *longPollKillPct {
				killAfter := rand.IntN(delay)
				log.Printf("will kill connection %d after %d ms (mid long poll)", count, killAfter)
				hj, ok := w.(http.Hijacker)
				if !ok {
					log.Printf("hijack not supported for request %d, dropping without response", count)
					return
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					log.Printf("hijack failed for request %d: %v", count, err)
					return
				}
				time.Sleep(time.Duration(killAfter) * time.Millisecond)
				log.Printf("killing connection %d during long poll", count)
				if tcpConn, ok := conn.(*net.TCPConn); ok {
					tcpConn.CloseWrite()
				} else {
					conn.Close()
				}
				return
			}

			time.Sleep(time.Duration(delay) * time.Millisecond)
			log.Printf("long poll complete for request %d, proxying now", count)
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

	srv := &http.Server{Addr: *listenAddr}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh
		log.Println("shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
	log.Println("shutdown complete")
}
