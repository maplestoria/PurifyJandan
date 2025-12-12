package main

import "encoding/json"

// RootResponse represents the top-level JSON structure.
type RootResponse struct {
	Code int        `json:"code"`
	Msg  string     `json:"msg"`
	Data *DataBlock `json:"data"`
}

// DataBlock represents the "data" object.
type DataBlock struct {
	Total       int    `json:"total"`
	TotalPages  int    `json:"total_pages"`
	CurrentPage int    `json:"current_page"`
	List        []Item `json:"list"`
}

// Item represents an entry in the "list" array.
type Item struct {
	ID      int    `json:"id"`
	Author  string `json:"author"`
	DateGMT string `json:"date_gmt"`
	Content string `json:"content"`
}

// DecodeRootResponse unmarshals bytes into RootResponse.
func DecodeRootResponse(b []byte) (*RootResponse, error) {
	var r RootResponse
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
