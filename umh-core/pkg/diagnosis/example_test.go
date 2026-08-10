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

package diagnosis_test

import (
	"fmt"
	"time"

	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/pkg/diagnosis"
)

// This example watches free disk space as HEADROOM above a 10 GB reserve, on two
// mounts. Headroom falls as a disk fills, so lower is worse: the signal fires at
// 0, where the reserve is first touched, and severity reaches 1 at -10, where the
// whole reserve is gone. Worst is NEGATIVE because it must lie on the worse side
// of Fire, which for a falling quantity is below it.
func Example() {
	type mounts struct{ root, varLog float64 } // GB free above the reserve

	spare := diagnosis.Marks{
		Unit:     "GB",
		Polarity: diagnosis.LowerIsWorse,
		Fire:     diagnosis.Mark{At: 0, Inclusive: true},
		Clear:    diagnosis.Mark{At: 2},
		Worst:    -10,
	}
	watch := func(name string, read func(mounts) float64) diagnosis.Signal[mounts] {
		return diagnosis.Signal[mounts]{
			Name:       name,
			DemoteSpan: time.Minute,
			Instruments: []diagnosis.Instrument[mounts]{{
				Name:    "spare",
				Red:     diagnosis.Last,
				Span:    10 * time.Second,
				Marks:   spare,
				Extract: func(m mounts) diagnosis.Reading { return diagnosis.Known(read(m)) },
			}},
		}
	}

	engine, err := diagnosis.NewEngine(diagnosis.Table[mounts]{
		Interval: time.Second,
		Signals: []diagnosis.Signal[mounts]{
			watch("/", func(m mounts) float64 { return m.root }),
			watch("/var", func(m mounts) float64 { return m.varLog }),
		},
	})
	if err != nil {
		panic(err)
	}

	fired, _ := engine.Observe(mounts{root: -3, varLog: -10}, diagnosis.NewEnvironment(), time.Unix(0, 0))
	for _, cause := range diagnosis.Rank(fired) {
		fmt.Printf("%-4s %+5.1f %s  severity %.2f\n",
			cause.Signal, cause.Value, cause.Marks.Unit, cause.Severity())
	}

	// Output:
	// /var -10.0 GB  severity 1.00
	// /     -3.0 GB  severity 0.30
}
