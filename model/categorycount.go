package model

import "time"

type CategoryCount struct {
	Category string `json:"category"`
	Total    int    `json:"total"`
}

type CategoryCountResponse struct {
	Status    string           `json:"status"`
	Total     int              `json:"total"`
	Advice    string           `json:"advice"`
	Timestamp time.Time        `json:"timestamp"`
	Items     []*CategoryCount `json:"items"`
}
