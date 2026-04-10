package model

import "time"

// Client representa un cliente registrado (instalación local). TokenHash no se expone en JSON.
type Client struct {
	ID        string    `json:"id"`
	TokenHash string    `json:"-"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ClientCreatedResponse se devuelve una sola vez al crear el cliente (incluye token en claro).
type ClientCreatedResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

// ClientUpdatedResponse permite devolver el token solo cuando se rota.
type ClientUpdatedResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Token     *string   `json:"token,omitempty"`
}
