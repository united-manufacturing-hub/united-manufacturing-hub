package services

import (
	"database/sql"
	"errors"
	"github.com/patrickmn/go-cache"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"strings"
	"sync"
	"time"
)

var treeCache = cache.New(1*time.Minute, 10*time.Minute)

const (
	cacheKeyFormatTree = "format-tree"
	cacheKeyValueTree  = "value-tree"
)

func GetFormatTreeStructureFromCache() (tree interface{}, err error) {
	zap.S().Infof("[GetFormatTreeStructureFromCache] Getting tree structure from cache")
	get, b := treeCache.Get(cacheKeyFormatTree)
	if b {
		zap.S().Infof("[GetFormatTreeStructureFromCache] Tree structure found in cache")
		go refreshFormatTreeCache()
		return get, nil
	}
	zap.S().Infof("[GetFormatTreeStructureFromCache] Tree structure not found in cache")
	tree, err = GetFormatTreeStructure()
	if err != nil {
		return
	}
	treeCache.Set(cacheKeyFormatTree, tree, cache.DefaultExpiration)

	return tree, nil
}

func GetValueTreeStructureFromCache(request models.GetTagGroupsRequest) (tree interface{}, err error) {
	zap.S().Infof("[GetValueTreeStructureFromCache] Getting tree structure from cache")
	get, b := treeCache.Get(cacheKeyValueTree)
	if b {
		zap.S().Infof("[GetValueTreeStructureFromCache] Tree structure found in cache")
		go refreshValueTreeCache(request)
		return get, nil
	}
	zap.S().Infof("[GetValueTreeStructureFromCache] Tree structure not found in cache")
	tree, err = GetValueTreeStructure(request)
	if err != nil {
		return
	}
	treeCache.Set(cacheKeyValueTree, tree, cache.DefaultExpiration)

	return tree, nil
}

var refreshRunningFT sync.Mutex

func refreshFormatTreeCache() {
	lockAcquired := refreshRunningFT.TryLock()
	if !lockAcquired {
		zap.S().Infof("[refreshFormatTreeCache] Tree structure refresh already running")
		return
	}

	structure, err := GetFormatTreeStructure()
	if err != nil {
		refreshRunningFT.Unlock()
		return
	}
	treeCache.Set(cacheKeyFormatTree, structure, cache.DefaultExpiration)
	refreshRunningFT.Unlock()
}

var refreshRunningVT sync.Mutex

func refreshValueTreeCache(request models.GetTagGroupsRequest) {
	lockAcquired := refreshRunningVT.TryLock()
	if !lockAcquired {
		zap.S().Infof("[refreshValueTreeCache] Tree structure refresh already running")
		return
	}

	structure, err := GetValueTreeStructure(request)
	if err != nil {
		refreshRunningVT.Unlock()
		return
	}
	treeCache.Set(cacheKeyValueTree, structure, cache.DefaultExpiration)
	refreshRunningVT.Unlock()
}

func GetValueTreeStructure(request models.GetTagGroupsRequest) (
	tree interface{},
	err error) {

	customer := request.EnterpriseName
	site := request.SiteName
	area := request.AreaName
	line := request.ProductionLineName
	workCell := request.WorkCellName

	var tables []models.TreeEntryFormat
	tables, err = GetTableTreeStructure(customer, site, area, line, workCell)
	if err != nil {
		return tree, err
	}
	var kpis []models.TreeEntryFormat
	kpis, err = GetKPITreeStructure(customer, site, area, line, workCell)
	if err != nil {
		return tree, err
	}
	var tags []models.TreeEntryFormat
	tags, err = GetTagsTreeStructure(customer, site, area, line, workCell)
	if err != nil {
		return tree, err
	}
	tree = models.TreeEntryValues{
		Label:  workCell,
		Value:  customer + "/" + site + "/" + area + "/" + line + "/" + workCell,
		Tables: tables,
		KPIs:   kpis,
		Tags:   tags,
	}

	return tree, nil
}

func GetFormatTreeStructure() (tree interface{}, err error) {

	var Entries []interface{}
	Entries, err = GetEnterpriseTreeStructure()
	if err != nil {
		return
	}

	return Entries, nil
}

func GetEnterpriseTreeStructure() (te []interface{}, err error) {

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

	for _, customer := range customers {
		var siteTree map[string]*models.TreeEntryFormat
		siteTree, err = GetSiteTreeStructure(customer)
		if err != nil {
			return nil, err
		}
		te = append(
			te, models.TreeEntryFormat{
				Label:   customer,
				Value:   customer,
				Entries: siteTree,
			})
	}
	return te, err
}

func GetSiteTreeStructure(customer string) (te map[string]*models.TreeEntryFormat, err error) {
	var sites []string
	te = make(map[string]*models.TreeEntryFormat)
	sites, err = GetSites(customer)
	if err != nil {
		return nil, err
	}
	for _, site := range sites {
		var areaTree map[string]*models.TreeEntryFormat
		areaTree, err = GetAreaTreeStructure(customer, site)
		if err != nil {
			return nil, err
		}
		value := customer + "/" + site
		te[site] = &models.TreeEntryFormat{
			Label:   site,
			Value:   value,
			Entries: areaTree,
		}
	}
	return te, err
}

func GetAreaTreeStructure(customer string, site string) (te map[string]*models.TreeEntryFormat, err error) {
	var areas []string
	te = make(map[string]*models.TreeEntryFormat)
	areas, err = GetAreas(customer, site)
	if err != nil {
		return nil, err
	}
	for _, area := range areas {
		var lineTree map[string]*models.TreeEntryFormat
		lineTree, err = GetLineTreeStructure(customer, site, area)
		if err != nil {
			return nil, err
		}
		value := customer + "/" + site + "/" + area
		te[area] = &models.TreeEntryFormat{
			Label:   area,
			Value:   value,
			Entries: lineTree,
		}
	}
	return te, err
}

func GetLineTreeStructure(customer string, site string, area string) (te map[string]*models.TreeEntryFormat, err error) {
	var lines []string
	te = make(map[string]*models.TreeEntryFormat)
	lines, err = GetProductionLines(customer, site, area)
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		var machineTree map[string]*models.TreeEntryFormat
		machineTree, err = GetWorkCellTreeStructure(customer, site, area, line)
		if err != nil {
			return nil, err
		}
		value := customer + "/" + site + "/" + area + "/" + line
		te[line] = &models.TreeEntryFormat{
			Label:   line,
			Value:   value,
			Entries: machineTree,
		}
	}
	return te, err
}

func GetWorkCellTreeStructure(customer string, site string, area string, line string) (
	te map[string]*models.TreeEntryFormat,
	err error) {
	var workCells []string
	te = make(map[string]*models.TreeEntryFormat)
	workCells, err = GetWorkCells(customer, site, area, line)
	if err != nil {
		return nil, err
	}
	for _, workCell := range workCells {
		value := customer + "/" + site + "/" + area + "/" + line + "/" + workCell
		te[workCell] = &models.TreeEntryFormat{
			Label: workCell,
			Value: value,
		}
	}
	return te, err
}

func GetTagsTreeStructure(customer string, site string, area string, line string, cell string) (
	te []models.TreeEntryFormat,
	err error) {

	zap.S().Infof("[GetTagsTreeStructure] customer: %s, site: %s, area: %s, line: %s, cell: %s", customer, site, area, line, cell)
	var tags []string
	tags, err = GetTagGroups(customer, site, area, line, cell)
	if err != nil {
		return nil, err
	}
	for _, tagGroup := range tags {
		var structure = make(map[string]*models.TreeEntryFormat)
		if tagGroup == models.StandardTagGroup {
			structure, err = GetStandardTagsTree(customer, site, area, line, cell, tagGroup)
		} else if tagGroup == models.CustomTagGroup {
			structure, err = GetCustomTagsTree(customer, site, area, line, cell, tagGroup)
		} else {
			err = errors.New("unknown tag group")
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		te = append(
			te, models.TreeEntryFormat{
				Label:   tagGroup,
				Value:   customer + "/" + site + "/" + area + "/" + line + "/" + cell + "/" + tagGroup,
				Entries: structure,
			})
	}
	return te, err
}

func GetCustomTagsTree(
	customer string,
	site string,
	area string,
	line string,
	cell string,
	group string) (te map[string]*models.TreeEntryFormat, err error) {

	te = make(map[string]*models.TreeEntryFormat)
	var id uint32
	id, err = GetWorkCellId(customer, site, cell)
	if err != nil {
		return nil, err
	}
	var tags []string
	tags, err = GetCustomTags(id)
	if err != nil {
		return nil, err
	}

	slices.Sort(tags)

	for _, tag := range tags {

		value := customer + "/" + site + "/" + area + "/" + line + "/" + cell + "/" + "tags" + "/" + group + "/" + tag

		// ignore tag that are only made of underscores
		if slices.Compact([]rune(tag))[0] == '_' {
			continue
		}

		splitTag := strings.Split(tag, "_")
		// ignore underscores at the beginning or end of the tag or if there are multiple underscores in a row
		sanitizedSplitTag := removeEmpty(splitTag)
		// remove unused capacity to save memory
		sanitizedSplitTag = slices.Clip(sanitizedSplitTag)

		// create the mapping
		te[splitTag[0]] = mapTagGrouping(te, splitTag, value)
	}

	return te, err
}

func mapTagGrouping(pte map[string]*models.TreeEntryFormat, st []string, value string) (a *models.TreeEntryFormat) {
	a, exists := pte[st[0]]
	// if the tag group does not exist, create it
	if !exists {
		a = &models.TreeEntryFormat{
			Label:   st[0],
			Value:   st[0],
			Entries: map[string]*models.TreeEntryFormat{},
		}
	}
	// if the tag group is a leaf, set value and end the recursion
	if len(st) > 1 {
		a.Entries[st[1]] = mapTagGrouping(a.Entries, st[1:], value)
	} else {
		a.Value = value
		a.Entries = nil
	}
	return
}

func removeEmpty(s []string) []string {
	var r []string
	idx := slices.Index(s, "")
	if idx != -1 {
		r = append(s[:idx], s[idx+1:]...)
		// recursively remove empty strings
		return removeEmpty(r)
	}
	return s
}

func GetStandardTagsTree(customer string, site string, area string, line string, cell string, tagGroup string) (
	te map[string]*models.TreeEntryFormat,
	err error) {

	te = make(map[string]*models.TreeEntryFormat)
	var tags []string
	tags, err = GetStandardTags(customer, site, cell)
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		value := customer + "/" + site + "/" + area + "/" + line + "/" + cell + "/" + "tags" + "/" + tagGroup + "/" + tag
		te[tag] = &models.TreeEntryFormat{
			Label: tag,
			Value: value,
		}
	}
	return te, err
}

func GetKPITreeStructure(customer string, site string, area string, line string, cell string) (
	te []models.TreeEntryFormat,
	err error) {

	zap.S().Debugf("[GetKPITreeStructure] customer: %s, site: %s, area: %s, line: %s, cell: %s", customer, site, area, line, cell)
	var kpis models.GetKpisMethodsResponse
	kpis, err = GetKpisMethods(customer, site, cell)
	if err != nil {
		return nil, err
	}
	for _, kpi := range kpis.Kpis {
		te = append(
			te, models.TreeEntryFormat{
				Label: kpi,
				Value: customer + "/" + site + "/" + area + "/" + line + "/" + cell + "/" + "kpis" + "/" + kpi,
			})
	}
	return te, err
}

func GetTableTreeStructure(customer string, site string, area string, line string, cell string) (
	te []models.TreeEntryFormat,
	err error) {

	zap.S().Debugf("[GetTableTreeStructure] customer: %s, site: %s, area: %s, line: %s, cell: %s", customer, site, area, line, cell)
	var types models.GetTableTypesResponse
	types, err = GetTableTypes(customer, site, cell)
	if err != nil {
		return nil, err
	}
	for _, table := range types.Tables {
		te = append(
			te, models.TreeEntryFormat{
				Label: table.Name,
				Value: customer + "/" + site + "/" + area + "/" + line + "/" + cell + "/" + "tables" + "/" + table.Name,
			})
	}
	return te, err
}

/*
// GetFormatTreeStructureFromCache returns the tree structure from cache and triggers a refresh in the background
func GetFormatTreeStructureFromCache() (tree models.TreeStructureEnterpriseMap, err error) {
	zap.S().Infof("[GetFormatTreeStructureFromCache] Getting tree structure from cache")
	get, b := treeCache.Get("tree")
	if b {
		zap.S().Infof("[GetFormatTreeStructureFromCache] Tree structure found in cache")
		go refreshFormatTreeCache()
		return get.(models.TreeStructureEnterpriseMap), nil
	}
	zap.S().Infof("[GetFormatTreeStructureFromCache] Tree structure not found in cache")
	tree, err = GetFormatTreeStructure()
	if err != nil {
		return
	}
	treeCache.Set("tree", tree, cache.DefaultExpiration)

	return tree, nil
}

var refreshRunningFT sync.Mutex

func refreshFormatTreeCache() {
	lockAcquired := refreshRunningFT.TryLock()
	if !lockAcquired {
		zap.S().Infof("[refreshFormatTreeCache] Tree structure refresh already running")
		return
	}

	structure, err := GetFormatTreeStructure()
	if err != nil {
		refreshRunningFT.Unlock()
		return
	}
	treeCache.Set("tree", structure, cache.DefaultExpiration)
	refreshRunningFT.Unlock()
}

func GetFormatTreeStructure() (tree models.TreeStructureEnterpriseMap, err error) {
	zap.S().Infof("[GetFormatTreeStructure] Getting tree structure")

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
	tst.Standard, err = GetStandardTags(enterprise, site, workCell)
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
	tx, err = GetTableTypes(customer, site, workCell)
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
*/
