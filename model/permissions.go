package model

type ClientPermissions struct {
	ClientID            string   `json:"client_id"`
	Restricted          bool     `json:"restricted"`
	MaxCreateCategories int      `json:"max_create_categories"`
	ReadCategories      []string `json:"read_categories"`
	WriteCategories     []string `json:"write_categories"`
	CreateCategories    []string `json:"create_categories"`
}

type UpsertClientPermissionsBody struct {
	Restricted          bool     `json:"restricted"`
	MaxCreateCategories int      `json:"max_create_categories"`
	ReadCategories      []string `json:"read_categories"`
	WriteCategories     []string `json:"write_categories"`
	CreateCategories    []string `json:"create_categories"`
}
