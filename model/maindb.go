package model

import (
	"time"

	"github.com/google/uuid"
)

type Maindb struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Category  string    `json:"category"`
	Subtype   string    `json:"subtype"`
	Data      string    `json:"data"`
	Available bool      `json:"available"`
}

// ApplyDefaults asigna un UUID si ID está vacío y la fecha/hora actual si CreatedAt no fue establecido.
// Llamalo antes de persistir (insert) desde el servicio o el store.
func (m *Maindb) ApplyDefaults() {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
}

// MaindbWithSync es un maindb con el flag de sincronización para el cliente autenticado.
type MaindbWithSync struct {
	Maindb
	Synced bool `json:"synced"`
}
