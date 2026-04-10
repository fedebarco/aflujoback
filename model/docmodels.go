package model

// ErrorJSON cuerpo JSON de error en endpoints que no usan model.Response (p. ej. POST /api/clients).
type ErrorJSON struct {
	Error string `json:"error"`
}
