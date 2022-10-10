package models

type Tag struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type GetTagsResponse struct {
	Tags []Tag `json:"tags"`
}
