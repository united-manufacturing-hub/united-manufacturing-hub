package models

type WorkCell struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetWorkCellsResponse struct {
	WorkCells []WorkCell `json:"workCells"`
}
