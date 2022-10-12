package services

import (
	"database/sql"
	"errors"
	"github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/database"
)

func GetEnterpriseId(enterpriseName string) (enterpriseId int, err error) {

	sqlStatement := `SELECT id FROM enterpriseTable WHERE name = $1`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, enterpriseName)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	} else if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("enterprise not found")
		return
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&enterpriseId)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}

func GetSiteId(enterpriseId int, siteName string) (siteId int, err error) {

	sqlStatement := `SELECT id FROM siteTable WHERE enterpriseId = $1 AND name = $2`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, enterpriseId, siteName)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	} else if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("site not found")
		return
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&siteId)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}

func GetAreaId(siteId int, areaName string) (areaId int, err error) {

	sqlStatement := `SELECT id FROM areaTable WHERE siteId = $1 AND name = $2`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, siteId, areaName)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	} else if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("area not found")
		return
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&areaId)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}

func GetProductionLineId(areaId int, productionLineName string) (productionLineId int, err error) {

	sqlStatement := `SELECT id FROM productionLineTable WHERE areaId = $1 AND name = $2`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, areaId, productionLineName)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	} else if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("production line not found")
		return
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&productionLineId)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}

func GetWorkCellId(productionLineId int, workCellName string) (workCellId int, err error) {

	sqlStatement := `SELECT id FROM workCellTable WHERE productionLineId = $1 AND name = $2`

	var rows *sql.Rows
	rows, err = db.Query(sqlStatement, productionLineId, workCellName)
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	} else if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("work cell not found")
		return
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&workCellId)
		if err != nil {
			database.ErrorHandling(sqlStatement, err, false)
			return
		}
	}
	err = rows.Err()
	if err != nil {
		database.ErrorHandling(sqlStatement, err, false)
		return
	}

	return
}
