package services

import (
	"database/sql"
	"errors"
	"github.com/patrickmn/go-cache"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
	"sync"
	"time"
)

var treeCache = cache.New(1*time.Minute, 10*time.Minute)

// GetTreeStructureFromCache returns the tree structure from cache and triggers a refresh in the background
func GetTreeStructureFromCache() (tree models.TreeStructureEnterpriseMap, err error) {
	zap.S().Infof("[GetTreeStructureFromCache] Getting tree structure from cache")
	get, b := treeCache.Get("tree")
	if b {
		zap.S().Infof("[GetTreeStructureFromCache] Tree structure found in cache")
		go refreshTreeCache()
		return get.(models.TreeStructureEnterpriseMap), nil
	}
	zap.S().Infof("[GetTreeStructureFromCache] Tree structure not found in cache")
	tree, err = GetTreeStructure()
	if err != nil {
		return
	}
	treeCache.Set("tree", tree, cache.DefaultExpiration)

	return tree, nil
}

var refreshRunning sync.Mutex

func refreshTreeCache() {
	lockAcquired := refreshRunning.TryLock()
	if !lockAcquired {
		zap.S().Infof("[refreshTreeCache] Tree structure refresh already running")
		return
	}

	structure, err := GetTreeStructure()
	if err != nil {
		refreshRunning.Unlock()
		return
	}
	treeCache.Set("tree", structure, cache.DefaultExpiration)
	refreshRunning.Unlock()
}

func GetTreeStructure() (tree models.TreeStructureEnterpriseMap, err error) {
	zap.S().Infof("[GetTreeStructure] Getting tree structure")

	sqlStatement := `SELECT DISTINCT customer FROM assetTable;`
	var rows *sql.Rows
	rows, err = database.Db.Query(sqlStatement)
	if errors.Is(err, sql.ErrNoRows) {
		// it can happen, no need to escalate error
		zap.S().Debugf("No Results Found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	defer rows.Close()

	var customers []string
	for rows.Next() {
		var customer string
		err = rows.Scan(&customer)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)

			return
		}
		customers = append(customers, customer)
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)

		return
	}

	var tse models.TreeStructureEnterpriseMap
	tse, err = GetTSE(customers)

	return tse, err
}

func GetTSE(customers []string) (tseMap models.TreeStructureEnterpriseMap, err error) {
	tseMap = make(models.TreeStructureEnterpriseMap)
	// Get sites for each customer
	for _, customer := range customers {
		var sites []string
		sites, err = GetSites(customer)
		if err != nil {
			continue
		}
		tseMap[customer] = models.TreeStructureEnterprise{
			Sites: make(map[string]models.TreeStructureSite),
		}
		// Get TSS for each site
		for _, site := range sites {
			var tss models.TreeStructureSite
			tss, err = GetTSS(customer, site)
			if err != nil {
				continue
			}
			tseMap[customer].Sites[site] = tss
		}
	}
	return
}

func GetTSS(customer string, site string) (models.TreeStructureSite, error) {
	tss := models.TreeStructureSite{}
	// Get TSA for each site
	areas, err := GetAreas(customer, site)
	if err != nil {
		return tss, err
	}
	tss.Areas = make(map[string]models.TreeStructureArea)
	for _, area := range areas {
		var tsa models.TreeStructureArea
		tsa, err = GetTSA(customer, site, area)
		if err != nil {
			continue
		}
		tss.Areas[area] = tsa
	}
	return tss, nil
}

func GetTSA(customer string, site string, area string) (models.TreeStructureArea, error) {
	tsa := models.TreeStructureArea{}
	// Get TSP for each area
	productionLines, err := GetProductionLines(customer, site, area)
	if err != nil {
		return tsa, err
	}
	tsa.ProductionLines = make(map[string]models.TreeStructureProductionLines)
	for _, productionLine := range productionLines {
		var tsp models.TreeStructureProductionLines
		tsp, err = GetTSP(customer, site, area, productionLine)
		if err != nil {
			continue
		}
		tsa.ProductionLines[productionLine] = tsp
	}
	return tsa, nil
}

func GetTSP(customer string, site string, area string, line string) (models.TreeStructureProductionLines, error) {
	tsp := models.TreeStructureProductionLines{}
	// Get TSW for each production line
	workCells, err := GetWorkCells(customer, site, area, line)
	if err != nil {
		return tsp, err
	}
	tsp.WorkCells = make(map[string]models.TreeStructureWorkCell)
	for _, workCell := range workCells {
		tsw := models.TreeStructureWorkCell{
			Tables: make(map[string]models.TreeStructureTables),
			KPIs:   make([]string, 0),
		}

		var tst map[string]models.TreeStructureTables
		tst, err = GetTST(customer, site, area, line, workCell)
		if err != nil {
			return models.TreeStructureProductionLines{}, err
		}

		tsw.Tables = tst

		var methods models.GetKpisMethodsResponse
		methods, err = GetKpisMethods(customer, site, workCell)
		if err != nil {
			return models.TreeStructureProductionLines{}, err
		}
		tsw.KPIs = append(tsw.KPIs, methods.Kpis...)

		tsw.Tags, err = GetTSTags(customer, site, workCell)
		if err != nil {
			return models.TreeStructureProductionLines{}, err
		}

		tsp.WorkCells[workCell] = tsw
	}
	return tsp, nil
}

func GetTSTags(enterprise string, site string, workCell string) (tst models.TreeStructureTags, err error) {
	tst = models.TreeStructureTags{}
	tst.Standard, err = GetStandardTags()
	if err != nil {
		return models.TreeStructureTags{}, err
	}

	var id uint32
	id, err = GetWorkCellId(enterprise, site, workCell)
	if err != nil {
		return models.TreeStructureTags{}, err
	}

	tst.Custom, err = GetCustomTags(id)

	return
}

func GetTST(customer string, site string, area string, line string, workCell string) (
	tables map[string]models.TreeStructureTables,
	err error) {
	var tx models.GetTableTypesResponse
	tx, err = GetTableTypes(customer, site, area, line, workCell)
	if err != nil {
		return
	}

	tables = make(map[string]models.TreeStructureTables)
	for _, table := range tx.Tables {
		tables[table.Name] = models.TreeStructureTables{
			Id: table.Id,
		}
	}
	return
}
