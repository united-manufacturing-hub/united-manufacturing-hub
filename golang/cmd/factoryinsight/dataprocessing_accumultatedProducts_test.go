package main

import (
	"database/sql"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/pkg/datamodel"
	"reflect"
	"testing"
	"time"
)

func TestCalculateAccumulatedProducts_Empty(t *testing.T) {

	var c *gin.Context
	to := time.Unix(0, 0)
	observationStart := time.Unix(0, 0)
	observationEnd := time.Unix(0, 0)

	countMap := make([]CountStruct, 0)
	orderMap := make([]OrderStruct, 0)
	productMap := make(map[int]ProductStruct)

	accumulatedProducts, err := CalculateAccumulatedProducts(c, to, observationStart, observationEnd, countMap, orderMap, productMap)
	if err != nil {
		t.Error(err)
	}

	var datapoints datamodel.DataResponseAny
	datapoints.ColumnNames = []string{
		"Target Output",
		"Actual Output",
		"Actual Scrap",
		"timestamp",
		"Internal Order ID",
		"Ordered Units",
		"Predicted Output",
		"Predicted Scrap",
		"Predicted Target",
		"Target Output after Order End",
		"Actual Output after Order End",
		"Actual Scrap after Order End",
		"Actual Good Output",
		"Actual Good Output after Order End",
		"Predicted Good Output",
	}
	datapoints.Datapoints = make([][]interface{}, 0)

	if !reflect.DeepEqual(accumulatedProducts, datapoints) {
		t.Error()
	}
}

func TestCalculateAccumulatedProducts_OrderWithoutCounts(t *testing.T) {
	var c *gin.Context
	to := time.Unix(200, 0)
	observationStart := time.Unix(10, 0)
	observationEnd := time.Unix(100, 0)

	countMap := make([]CountStruct, 0)
	orderMap := make([]OrderStruct, 0)
	productMap := make(map[int]ProductStruct)

	orderMap = append(orderMap, OrderStruct{
		orderID:        0,
		productId:      1,
		targetUnits:    10,
		beginTimeStamp: time.Unix(0, 0),
		endTimeStamp: sql.NullTime{
			Time:  time.Unix(10, 0),
			Valid: true,
		},
	})

	productMap[1] = ProductStruct{productId: 1, timePerProductUnitInSec: 1.0}

	accumulatedProducts, err := CalculateAccumulatedProducts(c, to, observationStart, observationEnd, countMap, orderMap, productMap)
	if err != nil {
		t.Error(err)
	}

	var datapoints datamodel.DataResponseAny
	datapoints.ColumnNames = []string{
		"Target Output",
		"Actual Output",
		"Actual Scrap",
		"timestamp",
		"Internal Order ID",
		"Ordered Units",
		"Predicted Output",
		"Predicted Scrap",
		"Predicted Target",
		"Target Output after Order End",
		"Actual Output after Order End",
		"Actual Scrap after Order End",
		"Actual Good Output",
		"Actual Good Output after Order End",
		"Predicted Good Output",
	}
	colLen := len(datapoints.ColumnNames)
	points := 2
	datapoints.Datapoints = make([][]interface{}, points)

	for i := 0; i < points; i++ {
		datapoints.Datapoints[i] = make([]interface{}, colLen)
	}

	datapoints.Datapoints[0][0] = int64(0)
	datapoints.Datapoints[0][1] = 0
	datapoints.Datapoints[0][2] = 0
	datapoints.Datapoints[0][3] = int64(10000)
	datapoints.Datapoints[0][4] = 0
	datapoints.Datapoints[0][5] = 0
	datapoints.Datapoints[0][6] = nil
	datapoints.Datapoints[0][7] = nil
	datapoints.Datapoints[0][8] = nil
	datapoints.Datapoints[0][9] = nil
	datapoints.Datapoints[0][10] = nil
	datapoints.Datapoints[0][11] = nil
	datapoints.Datapoints[0][12] = 0
	datapoints.Datapoints[0][13] = nil
	datapoints.Datapoints[0][14] = nil

	datapoints.Datapoints[1][0] = int64(0)
	datapoints.Datapoints[1][1] = 0
	datapoints.Datapoints[1][2] = 0
	datapoints.Datapoints[1][3] = int64(70000)
	datapoints.Datapoints[1][4] = 0
	datapoints.Datapoints[1][5] = 0
	datapoints.Datapoints[1][6] = nil
	datapoints.Datapoints[1][7] = nil
	datapoints.Datapoints[1][8] = nil
	datapoints.Datapoints[1][9] = nil
	datapoints.Datapoints[1][10] = nil
	datapoints.Datapoints[1][11] = nil
	datapoints.Datapoints[1][12] = 0
	datapoints.Datapoints[1][13] = nil
	datapoints.Datapoints[1][14] = nil

	if !reflect.DeepEqual(accumulatedProducts, datapoints) {
		for i, datapoint := range accumulatedProducts.Datapoints {
			fmt.Printf("[%d] %s\n", i, datapoint)
		}
		for i, datapoint := range datapoints.Datapoints {
			fmt.Printf("[%d] %s\n", i, datapoint)
		}
		t.Error()
	}
}

func TestCalculateAccumulatedProducts_OrderWithoutCountsWithPrediction(t *testing.T) {
	var c *gin.Context
	to := time.Unix(600, 0)
	observationStart := time.Unix(10, 0)
	observationEnd := time.Unix(200, 0)

	countMap := make([]CountStruct, 0)
	orderMap := make([]OrderStruct, 0)
	productMap := make(map[int]ProductStruct)

	orderMap = append(orderMap, OrderStruct{
		orderID:        0,
		productId:      1,
		targetUnits:    10,
		beginTimeStamp: time.Unix(0, 0),
		endTimeStamp: sql.NullTime{
			Time:  time.Unix(10, 0),
			Valid: true,
		},
	})

	productMap[1] = ProductStruct{productId: 1, timePerProductUnitInSec: 1.0}

	accumulatedProducts, err := CalculateAccumulatedProducts(c, to, observationStart, observationEnd, countMap, orderMap, productMap)
	if err != nil {
		t.Error(err)
	}

	var datapoints datamodel.DataResponseAny
	datapoints.ColumnNames = []string{
		"Target Output",
		"Actual Output",
		"Actual Scrap",
		"timestamp",
		"Internal Order ID",
		"Ordered Units",
		"Predicted Output",
		"Predicted Scrap",
		"Predicted Target",
		"Target Output after Order End",
		"Actual Output after Order End",
		"Actual Scrap after Order End",
		"Actual Good Output",
		"Actual Good Output after Order End",
		"Predicted Good Output",
	}
	colLen := len(datapoints.ColumnNames)
	points := 11
	datapoints.Datapoints = make([][]interface{}, points)

	for i := 0; i < points; i++ {
		datapoints.Datapoints[i] = make([]interface{}, colLen)
	}

	datapoints.Datapoints[0][0] = int64(0)
	datapoints.Datapoints[0][1] = 0
	datapoints.Datapoints[0][2] = 0
	datapoints.Datapoints[0][3] = int64(10000)
	datapoints.Datapoints[0][4] = 0
	datapoints.Datapoints[0][5] = 0
	datapoints.Datapoints[0][6] = nil
	datapoints.Datapoints[0][7] = nil
	datapoints.Datapoints[0][8] = nil
	datapoints.Datapoints[0][9] = nil
	datapoints.Datapoints[0][10] = nil
	datapoints.Datapoints[0][11] = nil
	datapoints.Datapoints[0][12] = 0
	datapoints.Datapoints[0][13] = nil
	datapoints.Datapoints[0][14] = nil

	datapoints.Datapoints[1][0] = int64(0)
	datapoints.Datapoints[1][1] = 0
	datapoints.Datapoints[1][2] = 0
	datapoints.Datapoints[1][3] = int64(70000)
	datapoints.Datapoints[1][4] = 0
	datapoints.Datapoints[1][5] = 0
	datapoints.Datapoints[1][6] = nil
	datapoints.Datapoints[1][7] = nil
	datapoints.Datapoints[1][8] = nil
	datapoints.Datapoints[1][9] = nil
	datapoints.Datapoints[1][10] = nil
	datapoints.Datapoints[1][11] = nil
	datapoints.Datapoints[1][12] = 0
	datapoints.Datapoints[1][13] = nil
	datapoints.Datapoints[1][14] = nil

	datapoints.Datapoints[2][0] = int64(0)
	datapoints.Datapoints[2][1] = 0
	datapoints.Datapoints[2][2] = 0
	datapoints.Datapoints[2][3] = int64(130000)
	datapoints.Datapoints[2][4] = 0
	datapoints.Datapoints[2][5] = 0
	datapoints.Datapoints[2][6] = nil
	datapoints.Datapoints[2][7] = nil
	datapoints.Datapoints[2][8] = nil
	datapoints.Datapoints[2][9] = nil
	datapoints.Datapoints[2][10] = nil
	datapoints.Datapoints[2][11] = nil
	datapoints.Datapoints[2][12] = 0
	datapoints.Datapoints[2][13] = nil
	datapoints.Datapoints[2][14] = nil

	datapoints.Datapoints[3][0] = int64(0)
	datapoints.Datapoints[3][1] = 0
	datapoints.Datapoints[3][2] = 0
	datapoints.Datapoints[3][3] = int64(190000)
	datapoints.Datapoints[3][4] = 0
	datapoints.Datapoints[3][5] = 0
	datapoints.Datapoints[3][6] = nil
	datapoints.Datapoints[3][7] = nil
	datapoints.Datapoints[3][8] = nil
	datapoints.Datapoints[3][9] = nil
	datapoints.Datapoints[3][10] = nil
	datapoints.Datapoints[3][11] = nil
	datapoints.Datapoints[3][12] = 0
	datapoints.Datapoints[3][13] = nil
	datapoints.Datapoints[3][14] = nil

	// Prediction points
	datapoints.Datapoints[4][0] = nil
	datapoints.Datapoints[4][1] = nil
	datapoints.Datapoints[4][2] = nil
	datapoints.Datapoints[4][3] = int64(190001)
	datapoints.Datapoints[4][4] = nil
	datapoints.Datapoints[4][5] = nil
	datapoints.Datapoints[4][6] = float64(0)
	datapoints.Datapoints[4][7] = float64(0)
	datapoints.Datapoints[4][8] = float64(0)
	datapoints.Datapoints[4][9] = int64(0)
	datapoints.Datapoints[4][10] = 0
	datapoints.Datapoints[4][11] = 0
	datapoints.Datapoints[4][12] = nil
	datapoints.Datapoints[4][13] = 0
	datapoints.Datapoints[4][14] = float64(0)

	datapoints.Datapoints[5][0] = nil
	datapoints.Datapoints[5][1] = nil
	datapoints.Datapoints[5][2] = nil
	datapoints.Datapoints[5][3] = int64(250001)
	datapoints.Datapoints[5][4] = nil
	datapoints.Datapoints[5][5] = nil
	datapoints.Datapoints[5][6] = float64(0)
	datapoints.Datapoints[5][7] = float64(0)
	datapoints.Datapoints[5][8] = float64(0)
	datapoints.Datapoints[5][9] = int64(0)
	datapoints.Datapoints[5][10] = 0
	datapoints.Datapoints[5][11] = 0
	datapoints.Datapoints[5][12] = nil
	datapoints.Datapoints[5][13] = 0
	datapoints.Datapoints[5][14] = float64(0)

	datapoints.Datapoints[6][0] = nil
	datapoints.Datapoints[6][1] = nil
	datapoints.Datapoints[6][2] = nil
	datapoints.Datapoints[6][3] = int64(310001)
	datapoints.Datapoints[6][4] = nil
	datapoints.Datapoints[6][5] = nil
	datapoints.Datapoints[6][6] = float64(0)
	datapoints.Datapoints[6][7] = float64(0)
	datapoints.Datapoints[6][8] = float64(0)
	datapoints.Datapoints[6][9] = int64(0)
	datapoints.Datapoints[6][10] = 0
	datapoints.Datapoints[6][11] = 0
	datapoints.Datapoints[6][12] = nil
	datapoints.Datapoints[6][13] = 0
	datapoints.Datapoints[6][14] = float64(0)

	datapoints.Datapoints[7][0] = nil
	datapoints.Datapoints[7][1] = nil
	datapoints.Datapoints[7][2] = nil
	datapoints.Datapoints[7][3] = int64(370001)
	datapoints.Datapoints[7][4] = nil
	datapoints.Datapoints[7][5] = nil
	datapoints.Datapoints[7][6] = float64(0)
	datapoints.Datapoints[7][7] = float64(0)
	datapoints.Datapoints[7][8] = float64(0)
	datapoints.Datapoints[7][9] = int64(0)
	datapoints.Datapoints[7][10] = 0
	datapoints.Datapoints[7][11] = 0
	datapoints.Datapoints[7][12] = nil
	datapoints.Datapoints[7][13] = 0
	datapoints.Datapoints[7][14] = float64(0)

	datapoints.Datapoints[8][0] = nil
	datapoints.Datapoints[8][1] = nil
	datapoints.Datapoints[8][2] = nil
	datapoints.Datapoints[8][3] = int64(430001)
	datapoints.Datapoints[8][4] = nil
	datapoints.Datapoints[8][5] = nil
	datapoints.Datapoints[8][6] = float64(0)
	datapoints.Datapoints[8][7] = float64(0)
	datapoints.Datapoints[8][8] = float64(0)
	datapoints.Datapoints[8][9] = int64(0)
	datapoints.Datapoints[8][10] = 0
	datapoints.Datapoints[8][11] = 0
	datapoints.Datapoints[8][12] = nil
	datapoints.Datapoints[8][13] = 0
	datapoints.Datapoints[8][14] = float64(0)

	datapoints.Datapoints[9][0] = nil
	datapoints.Datapoints[9][1] = nil
	datapoints.Datapoints[9][2] = nil
	datapoints.Datapoints[9][3] = int64(490001)
	datapoints.Datapoints[9][4] = nil
	datapoints.Datapoints[9][5] = nil
	datapoints.Datapoints[9][6] = float64(0)
	datapoints.Datapoints[9][7] = float64(0)
	datapoints.Datapoints[9][8] = float64(0)
	datapoints.Datapoints[9][9] = int64(0)
	datapoints.Datapoints[9][10] = 0
	datapoints.Datapoints[9][11] = 0
	datapoints.Datapoints[9][12] = nil
	datapoints.Datapoints[9][13] = 0
	datapoints.Datapoints[9][14] = float64(0)

	datapoints.Datapoints[10][0] = nil
	datapoints.Datapoints[10][1] = nil
	datapoints.Datapoints[10][2] = nil
	datapoints.Datapoints[10][3] = int64(550001)
	datapoints.Datapoints[10][4] = nil
	datapoints.Datapoints[10][5] = nil
	datapoints.Datapoints[10][6] = float64(0)
	datapoints.Datapoints[10][7] = float64(0)
	datapoints.Datapoints[10][8] = float64(0)
	datapoints.Datapoints[10][9] = int64(0)
	datapoints.Datapoints[10][10] = 0
	datapoints.Datapoints[10][11] = 0
	datapoints.Datapoints[10][12] = nil
	datapoints.Datapoints[10][13] = 0
	datapoints.Datapoints[10][14] = float64(0)

	if !reflect.DeepEqual(accumulatedProducts, datapoints) {
		for i, datapoint := range accumulatedProducts.Datapoints {
			fmt.Printf("[%d] %s\n", i, datapoint)
		}
		for i, datapoint := range datapoints.Datapoints {
			fmt.Printf("[%d] %s\n", i, datapoint)
		}
		t.Error()
	}
}

func TestCalculateAccumulatedProducts_OrderWithCountsSimple(t *testing.T) {
	var c *gin.Context
	to := time.Unix(200, 0)
	observationStart := time.Unix(10, 0)
	observationEnd := time.Unix(100, 0)

	countMap := make([]CountStruct, 0)
	orderMap := make([]OrderStruct, 0)
	productMap := make(map[int]ProductStruct)

	orderMap = append(orderMap, OrderStruct{
		orderID:        0,
		productId:      1,
		targetUnits:    10,
		beginTimeStamp: time.Unix(0, 0),
		endTimeStamp: sql.NullTime{
			Time:  time.Unix(10, 0),
			Valid: true,
		},
	})

	productMap[1] = ProductStruct{productId: 1, timePerProductUnitInSec: 1.0}

	countMap = append(countMap, CountStruct{
		timestamp: time.Unix(10, 0),
		count:     100,
		scrap:     3,
	})

	countMap = append(countMap, CountStruct{
		timestamp: time.Unix(70, 0),
		count:     100,
		scrap:     3,
	})

	countMap = append(countMap, CountStruct{
		timestamp: time.Unix(80, 0),
		count:     200,
		scrap:     6,
	})

	accumulatedProducts, err := CalculateAccumulatedProducts(c, to, observationStart, observationEnd, countMap, orderMap, productMap)
	if err != nil {
		t.Error(err)
	}

	var datapoints datamodel.DataResponseAny
	datapoints.ColumnNames = []string{
		"Target Output",
		"Actual Output",
		"Actual Scrap",
		"timestamp",
		"Internal Order ID",
		"Ordered Units",
		"Predicted Output",
		"Predicted Scrap",
		"Predicted Target",
		"Target Output after Order End",
		"Actual Output after Order End",
		"Actual Scrap after Order End",
		"Actual Good Output",
		"Actual Good Output after Order End",
		"Predicted Good Output",
	}
	colLen := len(datapoints.ColumnNames)
	points := 2
	datapoints.Datapoints = make([][]interface{}, points)

	for i := 0; i < points; i++ {
		datapoints.Datapoints[i] = make([]interface{}, colLen)
	}

	datapoints.Datapoints[0][0] = int64(0)
	datapoints.Datapoints[0][1] = 100 //Products
	datapoints.Datapoints[0][2] = 3   //Scrap
	datapoints.Datapoints[0][3] = int64(10000)
	datapoints.Datapoints[0][4] = 0
	datapoints.Datapoints[0][5] = 0
	datapoints.Datapoints[0][6] = nil
	datapoints.Datapoints[0][7] = nil
	datapoints.Datapoints[0][8] = nil
	datapoints.Datapoints[0][9] = nil
	datapoints.Datapoints[0][10] = nil
	datapoints.Datapoints[0][11] = nil
	datapoints.Datapoints[0][12] = 97 //Good Products
	datapoints.Datapoints[0][13] = nil
	datapoints.Datapoints[0][14] = nil

	datapoints.Datapoints[1][0] = int64(0)
	datapoints.Datapoints[1][1] = 400
	datapoints.Datapoints[1][2] = 12
	datapoints.Datapoints[1][3] = int64(70000)
	datapoints.Datapoints[1][4] = 0
	datapoints.Datapoints[1][5] = 0
	datapoints.Datapoints[1][6] = nil
	datapoints.Datapoints[1][7] = nil
	datapoints.Datapoints[1][8] = nil
	datapoints.Datapoints[1][9] = nil
	datapoints.Datapoints[1][10] = nil
	datapoints.Datapoints[1][11] = nil
	datapoints.Datapoints[1][12] = 388
	datapoints.Datapoints[1][13] = nil
	datapoints.Datapoints[1][14] = nil

	if !reflect.DeepEqual(accumulatedProducts, datapoints) {
		for i, datapoint := range accumulatedProducts.Datapoints {
			fmt.Printf("[%d] %s\n", i, datapoint)
		}
		for i, datapoint := range datapoints.Datapoints {
			fmt.Printf("[%d] %s\n", i, datapoint)
		}
		t.Error()
	}
}

func TestCalculateAccumulatedProducts_OrderWithCountsSimplePredict(t *testing.T) {
	var c *gin.Context
	to := time.Unix(500, 0)
	observationStart := time.Unix(10, 0)
	observationEnd := time.Unix(200, 0)

	countMap := make([]CountStruct, 0)
	orderMap := make([]OrderStruct, 0)
	productMap := make(map[int]ProductStruct)

	orderMap = append(orderMap, OrderStruct{
		orderID:        0,
		productId:      1,
		targetUnits:    10,
		beginTimeStamp: time.Unix(0, 0),
		endTimeStamp: sql.NullTime{
			Time:  time.Unix(10, 0),
			Valid: true,
		},
	})

	productMap[1] = ProductStruct{productId: 1, timePerProductUnitInSec: 1.0}

	countMap = append(countMap, CountStruct{
		timestamp: time.Unix(10, 0),
		count:     100,
		scrap:     3,
	})

	countMap = append(countMap, CountStruct{
		timestamp: time.Unix(70, 0),
		count:     100,
		scrap:     3,
	})

	accumulatedProducts, err := CalculateAccumulatedProducts(c, to, observationStart, observationEnd, countMap, orderMap, productMap)
	if err != nil {
		t.Error(err)
	}

	var datapoints datamodel.DataResponseAny
	datapoints.ColumnNames = []string{
		"Target Output",
		"Actual Output",
		"Actual Scrap",
		"timestamp",
		"Internal Order ID",
		"Ordered Units",
		"Predicted Output",
		"Predicted Scrap",
		"Predicted Target",
		"Target Output after Order End",
		"Actual Output after Order End",
		"Actual Scrap after Order End",
		"Actual Good Output",
		"Actual Good Output after Order End",
		"Predicted Good Output",
	}
	colLen := len(datapoints.ColumnNames)
	points := 10
	datapoints.Datapoints = make([][]interface{}, points)

	for i := 0; i < points; i++ {
		datapoints.Datapoints[i] = make([]interface{}, colLen)
	}

	var i = 0
	datapoints.Datapoints[i][0] = int64(0)
	datapoints.Datapoints[i][1] = 100 //Products
	datapoints.Datapoints[i][2] = 3   //Scrap
	datapoints.Datapoints[i][3] = int64(10000)
	datapoints.Datapoints[i][4] = 0
	datapoints.Datapoints[i][5] = 0
	datapoints.Datapoints[i][6] = nil
	datapoints.Datapoints[i][7] = nil
	datapoints.Datapoints[i][8] = nil
	datapoints.Datapoints[i][9] = nil
	datapoints.Datapoints[i][10] = nil
	datapoints.Datapoints[i][11] = nil
	datapoints.Datapoints[i][12] = 97 //Good Products
	datapoints.Datapoints[i][13] = nil
	datapoints.Datapoints[i][14] = nil

	i += 1
	datapoints.Datapoints[i][0] = int64(0)
	datapoints.Datapoints[i][1] = 200
	datapoints.Datapoints[i][2] = 6
	datapoints.Datapoints[i][3] = int64(70000)
	datapoints.Datapoints[i][4] = 0
	datapoints.Datapoints[i][5] = 0
	datapoints.Datapoints[i][6] = nil
	datapoints.Datapoints[i][7] = nil
	datapoints.Datapoints[i][8] = nil
	datapoints.Datapoints[i][9] = nil
	datapoints.Datapoints[i][10] = nil
	datapoints.Datapoints[i][11] = nil
	datapoints.Datapoints[i][12] = 194
	datapoints.Datapoints[i][13] = nil
	datapoints.Datapoints[i][14] = nil

	i += 1
	datapoints.Datapoints[i][0] = int64(0)
	datapoints.Datapoints[i][1] = 200
	datapoints.Datapoints[i][2] = 6
	datapoints.Datapoints[i][3] = int64(130000)
	datapoints.Datapoints[i][4] = 0
	datapoints.Datapoints[i][5] = 0
	datapoints.Datapoints[i][6] = nil
	datapoints.Datapoints[i][7] = nil
	datapoints.Datapoints[i][8] = nil
	datapoints.Datapoints[i][9] = nil
	datapoints.Datapoints[i][10] = nil
	datapoints.Datapoints[i][11] = nil
	datapoints.Datapoints[i][12] = 194
	datapoints.Datapoints[i][13] = nil
	datapoints.Datapoints[i][14] = nil

	i += 1
	datapoints.Datapoints[i][0] = int64(0)
	datapoints.Datapoints[i][1] = 200
	datapoints.Datapoints[i][2] = 6
	datapoints.Datapoints[i][3] = int64(190000)
	datapoints.Datapoints[i][4] = 0
	datapoints.Datapoints[i][5] = 0
	datapoints.Datapoints[i][6] = nil
	datapoints.Datapoints[i][7] = nil
	datapoints.Datapoints[i][8] = nil
	datapoints.Datapoints[i][9] = nil
	datapoints.Datapoints[i][10] = nil
	datapoints.Datapoints[i][11] = nil
	datapoints.Datapoints[i][12] = 194
	datapoints.Datapoints[i][13] = nil
	datapoints.Datapoints[i][14] = nil

	i += 1
	datapoints.Datapoints[i][0] = nil
	datapoints.Datapoints[i][1] = nil
	datapoints.Datapoints[i][2] = nil
	datapoints.Datapoints[i][3] = int64(190001)
	datapoints.Datapoints[i][4] = nil
	datapoints.Datapoints[i][5] = nil
	datapoints.Datapoints[i][6] = float64(200)
	datapoints.Datapoints[i][7] = float64(6)
	datapoints.Datapoints[i][8] = float64(0)
	datapoints.Datapoints[i][9] = int64(0)
	datapoints.Datapoints[i][10] = 200
	datapoints.Datapoints[i][11] = 6
	datapoints.Datapoints[i][12] = nil
	datapoints.Datapoints[i][13] = 194
	datapoints.Datapoints[i][14] = float64(194)

	i += 1
	datapoints.Datapoints[i][0] = nil
	datapoints.Datapoints[i][1] = nil
	datapoints.Datapoints[i][2] = nil
	datapoints.Datapoints[i][3] = int64(250001)
	datapoints.Datapoints[i][4] = nil
	datapoints.Datapoints[i][5] = nil
	datapoints.Datapoints[i][6] = float64(261.4285714285714)
	datapoints.Datapoints[i][7] = float64(7.842857142857142)
	datapoints.Datapoints[i][8] = float64(0)
	datapoints.Datapoints[i][9] = int64(0)
	datapoints.Datapoints[i][10] = 200
	datapoints.Datapoints[i][11] = 6
	datapoints.Datapoints[i][12] = nil
	datapoints.Datapoints[i][13] = 194
	datapoints.Datapoints[i][14] = float64(253.58571428571423)

	i += 1
	datapoints.Datapoints[i][0] = nil
	datapoints.Datapoints[i][1] = nil
	datapoints.Datapoints[i][2] = nil
	datapoints.Datapoints[i][3] = int64(310001)
	datapoints.Datapoints[i][4] = nil
	datapoints.Datapoints[i][5] = nil
	datapoints.Datapoints[i][6] = float64(322.85714285714283)
	datapoints.Datapoints[i][7] = float64(9.685714285714285)
	datapoints.Datapoints[i][8] = float64(0)
	datapoints.Datapoints[i][9] = int64(0)
	datapoints.Datapoints[i][10] = 200
	datapoints.Datapoints[i][11] = 6
	datapoints.Datapoints[i][12] = nil
	datapoints.Datapoints[i][13] = 194
	datapoints.Datapoints[i][14] = float64(313.1714285714285)

	i += 1
	datapoints.Datapoints[i][0] = nil
	datapoints.Datapoints[i][1] = nil
	datapoints.Datapoints[i][2] = nil
	datapoints.Datapoints[i][3] = int64(370001)
	datapoints.Datapoints[i][4] = nil
	datapoints.Datapoints[i][5] = nil
	datapoints.Datapoints[i][6] = float64(384.2857142857143)
	datapoints.Datapoints[i][7] = float64(11.528571428571428)
	datapoints.Datapoints[i][8] = float64(0)
	datapoints.Datapoints[i][9] = int64(0)
	datapoints.Datapoints[i][10] = 200
	datapoints.Datapoints[i][11] = 6
	datapoints.Datapoints[i][12] = nil
	datapoints.Datapoints[i][13] = 194
	datapoints.Datapoints[i][14] = float64(372.75714285714287)

	i += 1
	datapoints.Datapoints[i][0] = nil
	datapoints.Datapoints[i][1] = nil
	datapoints.Datapoints[i][2] = nil
	datapoints.Datapoints[i][3] = int64(430001)
	datapoints.Datapoints[i][4] = nil
	datapoints.Datapoints[i][5] = nil
	datapoints.Datapoints[i][6] = float64(445.71428571428567)
	datapoints.Datapoints[i][7] = float64(13.37142857142857)
	datapoints.Datapoints[i][8] = float64(0)
	datapoints.Datapoints[i][9] = int64(0)
	datapoints.Datapoints[i][10] = 200
	datapoints.Datapoints[i][11] = 6
	datapoints.Datapoints[i][12] = nil
	datapoints.Datapoints[i][13] = 194
	datapoints.Datapoints[i][14] = float64(432.3428571428571)

	i += 1
	datapoints.Datapoints[i][0] = nil
	datapoints.Datapoints[i][1] = nil
	datapoints.Datapoints[i][2] = nil
	datapoints.Datapoints[i][3] = int64(490001)
	datapoints.Datapoints[i][4] = nil
	datapoints.Datapoints[i][5] = nil
	datapoints.Datapoints[i][6] = float64(507.1428571428571)
	datapoints.Datapoints[i][7] = float64(15.214285714285715)
	datapoints.Datapoints[i][8] = float64(0)
	datapoints.Datapoints[i][9] = int64(0)
	datapoints.Datapoints[i][10] = 200
	datapoints.Datapoints[i][11] = 6
	datapoints.Datapoints[i][12] = nil
	datapoints.Datapoints[i][13] = 194
	datapoints.Datapoints[i][14] = float64(491.9285714285714)

	if !reflect.DeepEqual(accumulatedProducts, datapoints) {
		for i, datapoint := range accumulatedProducts.Datapoints {
			fmt.Printf("[%d] %s\n", i, datapoint)
		}
		for i, datapoint := range datapoints.Datapoints {
			fmt.Printf("[%d] %s\n", i, datapoint)
		}
		t.Error()
	}
}
