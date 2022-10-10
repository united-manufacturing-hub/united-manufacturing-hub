package models

type TagGroup struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetTagGroupsResponse struct {
	TagGroups []TagGroup `json:"tagGroups"`
}
