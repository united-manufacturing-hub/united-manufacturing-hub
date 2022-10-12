package services

import (
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"
)

func GetKpis(enterpriseName string, siteName string, areaName string, productionLineName string, workCellName string) (kpis models.GetKpisResponse, err error) {

	enterpriseId, err := GetEnterpriseId(enterpriseName)
	if err != nil {
		return
	}
	siteId, err := GetSiteId(enterpriseId, siteName)
	if err != nil {
		return
	}
	areaId, err := GetAreaId(siteId, areaName)
	if err != nil {
		return
	}
	productionLineId, err := GetProductionLineId(areaId, productionLineName)
	if err != nil {
		return
	}
	workCellId, err := GetWorkCellId(productionLineId, workCellName)
	if err != nil {
		return
	}

	sqlStatement := `SELECT EXISTS(SELECT 1 FROM stateTable WHERE workCellId = $1)`

	var stateExists bool
	err = db.QueryRow(sqlStatement, workCellId).Scan(&stateExists)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	if stateExists {
		kpis.Kpis = append(kpis.Kpis, models.Kpi{Id: 1, Name: models.OeeKpi})
		kpis.Kpis = append(kpis.Kpis, models.Kpi{Id: 2, Name: models.AvailabilityKpi})
		kpis.Kpis = append(kpis.Kpis, models.Kpi{Id: 3, Name: models.PerformanceKpi})
		kpis.Kpis = append(kpis.Kpis, models.Kpi{Id: 4, Name: models.QualityKpi})
	}

	return
}
