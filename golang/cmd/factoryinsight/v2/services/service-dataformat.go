package services

import "github.com/united-manufacturing-hub/united-manufacturing-hub/cmd/factoryinsight/v2/models"

func GetDataFormats() (dataFormat models.GetDataFormatResponse, err error) {

	return models.DefaultDataFormats, nil
}
