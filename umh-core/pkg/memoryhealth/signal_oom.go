// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memoryhealth

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/umh-core/fsmv2/diagnosis"
)

const (
	signalOomKill             = "oom-kill"
	instrumentOomKillsLast60s = "oom-kills-last-60s"
	unitKills                 = "kills"
)

var oomKillMarks = diagnosis.Marks{
	Unit:     unitKills,
	Fire:     diagnosis.Mark{At: 1, Inclusive: true},
	Clear:    diagnosis.Mark{At: 0, Inclusive: true},
	Polarity: diagnosis.HigherIsWorse,
	Worst:    10,
}

func oomKillSignal(sum diagnosis.Reduction) diagnosis.Signal[Sample] {
	return diagnosis.Signal[Sample]{
		Name:            signalOomKill,
		Tier:            tierOutOfMemory,
		Attribution:     blameContainer,
		DemoteSpan:      averageSpan,
		ReleaseOnAbsent: true,
		Instruments: []diagnosis.Instrument[Sample]{{
			Measurement: diagnosis.Measurement[Sample]{
				Name:      instrumentOomKillsLast60s,
				Extract:   func(sample Sample) diagnosis.Reading { return sample.NewOomKills },
				Span:      averageSpan,
				Reduction: sum,
			},
			Marks: oomKillMarks,
		}},
	}
}
