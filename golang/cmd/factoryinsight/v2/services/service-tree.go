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
		var structure map[string]*models.TreeEntryFormat
		if tagGroup == models.StandardTagGroup {
			structure, err = GetStandardTagsTree(customer, site, area, line, cell, tagGroup)
		} else if tagGroup == models.CustomTagGroup {
			structure, err = GetCustomTagsTree(customer, site, area, line, cell, tagGroup, false)
		} else if tagGroup == models.CustomStringTagGroup {
			structure, err = GetCustomTagsTree(customer, site, area, line, cell, tagGroup, true)
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

func GetCustomTagsTree(customer string, site string, area string, line string, cell string, group string, isProcessValueStrings bool) (te map[string]*models.TreeEntryFormat, err error) {

	te = make(map[string]*models.TreeEntryFormat)
	var id uint32
	id, err = GetWorkCellId(customer, site, cell)
	if err != nil {
		return nil, err
	}
	var tags []string
	tags, err = GetCustomTags(id, isProcessValueStrings)
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
		te[splitTag[0]] = mapTagGrouping(te, sanitizedSplitTag, value)
	}

	return te, err
}

func mapTagGrouping(pte map[string]*models.TreeEntryFormat, st []string, value string) (a *models.TreeEntryFormat) {
	a, exists := pte[st[0]]
	zap.S().Debugf("mapTagGrouping: %s, %s, %s, %t", st[0], st[1:], value, exists)
	// if the tag group does not exist, create it
	if !exists {
		a = &models.TreeEntryFormat{
			Label:   st[0],
			Value:   st[0],
			Entries: make(map[string]*models.TreeEntryFormat),
		}
	}
	// if the tag group is a leaf, set value and end the recursion
	if len(st) > 1 {
		zap.S().Debugf("len(st) > 1: %s, %s, %s", st[0], st[1:], value)
		if a.Entries == nil {
			a.Entries = make(map[string]*models.TreeEntryFormat)
		}
		a.Entries[st[1]] = mapTagGrouping(a.Entries, st[1:], value)
	} else {
		zap.S().Debugf("len(st) <= 1: %s, %s, %s", st[0], st[1:], value)
		a.Value = value
		a.Entries = nil
	}
	return
}

func removeEmpty(s []string) []string {
	var r []string
	if idx := slices.Index(s, ""); idx != -1 {
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
