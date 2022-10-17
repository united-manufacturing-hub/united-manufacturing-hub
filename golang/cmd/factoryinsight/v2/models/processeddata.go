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
