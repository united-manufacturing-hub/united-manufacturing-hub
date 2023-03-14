// Copyright 2023 UMH Systems GmbH
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

package models

import (
	"database/sql"
	"time"
)

type CountStruct struct {
	Timestamp time.Time
	Count     int
	Scrap     int
}
type OrderStruct struct {
	BeginTimeStamp time.Time
	EndTimeStamp   sql.NullTime
	OrderID        int
	ProductId      int
	TargetUnits    int
}

type ProductStruct struct {
	ProductId               int
	TimePerProductUnitInSec float64
}

type ChannelResult struct {
	Err         error
	ReturnValue interface{}
}
