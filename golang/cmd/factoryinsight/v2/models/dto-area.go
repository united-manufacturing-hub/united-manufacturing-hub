package models

type Area struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetAreasResponse struct {
	Areas []Area `json:"areas"`
}
