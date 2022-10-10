package models

type WorkCell struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetWorkCellsResponse struct {
	Workcells []WorkCell `json:"workcells"`
}
