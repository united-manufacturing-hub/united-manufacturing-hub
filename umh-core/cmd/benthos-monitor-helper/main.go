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

// benthos-monitor-helper replaces the shell pipeline (curl | gzip -c | xxd -p)
// with a single long-lived process. S6 runs this as the monitor command.
//
// Each iteration fetches /ping, /ready, /version, /metrics from the local
// benthos instance, gzip-compresses and hex-encodes each response, and prints
// it between the same markers that the parser expects.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Marker constants — duplicated here to avoid importing the full
// benthos_monitor package and its transitive dependencies.
const (
	blockStartMarker = "BEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGINBEGIN"
	pingEndMarker    = "PINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGENDPINGEND"
	readyEnd         = "CONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGENDCONFIGEND"
	versionEnd       = "VERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONENDVERSIONEND"
	metricsEndMarker = "METRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSENDMETRICSEND"
	blockEndMarker   = "ENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDENDEND"
)

type endpoint struct {
	path      string
	endMarker string
}

var endpoints = []endpoint{
	{"/ping", pingEndMarker},
	{"/ready", readyEnd},
	{"/version", versionEnd},
	{"/metrics", metricsEndMarker},
}

func main() {
	port := flag.Int("port", 0, "benthos metrics port")
	flag.Parse()

	if *port == 0 {
		fmt.Fprintf(os.Stderr, "benthos-monitor-helper: --port is required\n")
		os.Exit(1)
	}

	offset := float64(*port%1000) / 1000.0
	client := &http.Client{Timeout: 1 * time.Second}

	w := bufio.NewWriter(os.Stdout)
	for {
		fmt.Fprintln(w, blockStartMarker)
		for _, ep := range endpoints {
			fetchAndEncode(w, client, *port, ep.path)
			fmt.Fprintln(w, ep.endMarker)
		}
		fmt.Fprintln(w, time.Now().UnixNano())
		fmt.Fprintln(w, blockEndMarker)
		w.Flush()

		sleepUntilOffset(offset)
	}
}

func fetchAndEncode(w *bufio.Writer, client *http.Client, port int, path string) {
	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	resp, err := client.Get(url)

	var data []byte
	if err != nil {
		data = []byte(err.Error())
	} else {
		data, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	// gzip + hex encode
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write(data)
	gz.Close()
	hex.NewEncoder(w).Write(buf.Bytes())
	fmt.Fprintln(w) // newline after hex data
}

func sleepUntilOffset(offset float64) {
	now := time.Now()
	frac := float64(now.Nanosecond()) / 1e9
	wait := offset - frac
	if wait <= 0.05 {
		wait += 1.0
	}
	time.Sleep(time.Duration(wait * float64(time.Second)))
}
