package services

import (
	"database/sql"
	"errors"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/internal"
	"go.uber.org/zap"
)

func GetEnterpriseId(enterpriseName string) (enterpriseId uint32, err error) {
	zap.S().Infof("[GetEnterpriseID] enterpriseName: %v", enterpriseName)

	// Get from cache if possible
	var cacheHit bool
	enterpriseId, cacheHit = internal.GetEnterpriseIDFromCache(enterpriseName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM enterpriseTable WHERE name = $1`

	err = db.QueryRow(sqlStatement, enterpriseName).Scan(&enterpriseId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetEnterpriseID] enterprise not found: %v", enterpriseName)
		err = errors.New("enterprise not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreEnterpriseIDToCache(enterpriseName, enterpriseId)
	return
}

func GetSiteId(enterpriseId uint32, siteName string) (siteId uint32, err error) {
	zap.S().Infof("[GetSiteID] enterpriseId: %v, siteName: %v", enterpriseId, siteName)

	// Get from cache if possible
	var cacheHit bool
	siteId, cacheHit = internal.GetSiteIDFromCache(enterpriseId, siteName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM siteTable WHERE enterpriseId = $1 AND name = $2`

	err = db.QueryRow(sqlStatement, enterpriseId, siteName).Scan(&siteId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetSiteID] site not found: %v", siteName)
		err = errors.New("site not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreSiteIDToCache(enterpriseId, siteName, siteId)
	return
}

func GetAreaId(siteId uint32, areaName string) (areaId uint32, err error) {
	zap.S().Infof("[GetAreaID] siteId: %v, areaName: %v", siteId, areaName)

	// Get from cache if possible
	var cacheHit bool
	areaId, cacheHit = internal.GetAreaIDFromCache(siteId, areaName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM areaTable WHERE siteId = $1 AND name = $2`

	err = db.QueryRow(sqlStatement, siteId, areaName).Scan(&areaId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetAreaID] area not found: %v", areaName)
		err = errors.New("area not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreAreaIDToCache(siteId, areaName, areaId)
	return
}

func GetProductionLineId(areaId uint32, productionLineName string) (productionLineId uint32, err error) {
	zap.S().Infof("[GetProductionLineID] areaId: %v, productionLineName: %v", areaId, productionLineName)

	// Get from cache if possible
	var cacheHit bool
	productionLineId, cacheHit = internal.GetProductionLineIDFromCache(areaId, productionLineName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM productionLineTable WHERE areaId = $1 AND name = $2`

	err = db.QueryRow(sqlStatement, areaId, productionLineName).Scan(&productionLineId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetProductionLineID] production line not found: %v", productionLineName)
		err = errors.New("production line not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreProductionLineIDToCache(areaId, productionLineName, productionLineId)
	return
}

func GetWorkCellId(productionLineId uint32, workCellName string) (workCellId uint32, err error) {
	zap.S().Infof("[GetWorkCellID] productionLineId: %v, workCellName: %v", productionLineId, workCellName)

	// Get from cache if possible
	var cacheHit bool
	workCellId, cacheHit = internal.GetWorkCellIDFromCache(productionLineId, workCellName)
	if cacheHit {
		return
	}

	sqlStatement := `SELECT id FROM workCellTable WHERE productionLineId = $1 AND name = $2`

	err = db.QueryRow(sqlStatement, productionLineId, workCellName).Scan(&workCellId)
	if errors.Is(err, sql.ErrNoRows) {
		database.ErrorHandling(sqlStatement, err, false)
		zap.S().Warnf("[GetWorkCellID] work cell not found: %v", workCellName)
		err = errors.New("work cell not found")
		return
	} else if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	// Store to cache if not yet existing
	go internal.StoreWorkCellIDToCache(productionLineId, workCellName, workCellId)
	return
}
