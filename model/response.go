package model

import (
	"time"
)

type Response struct {
	Status    string        `json:"status"`
	Total     int           `json:"total"`
	Advice    string        `json:"advice"`
	Timestamp time.Time     `json:"timestamp"`
	Items     []*Maindb `json:"items"`
}