package models

type DataFormat struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetDataFormatResponse struct {
	DataFormats []DataFormat `json:"dataFormats"`
}

var DefaultDataFormats = GetDataFormatResponse{
	DataFormats: []DataFormat{
		{Id: 1, Name: "Tags"},
		{Id: 2, Name: "KPIs"},
		{Id: 3, Name: "Lists"},
	},
}
