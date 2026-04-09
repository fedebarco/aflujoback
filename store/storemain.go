package store

import (
	"database/sql"
	"time"
	"aflujo/model"
)

// Storer define el contrato de persistencia para Maindb.
type Storer interface {
	GetAll() ([]*model.Maindb, error)
	GetByID(id string) (*model.Maindb, error)
	GetByCategory(category string) ([]*model.Maindb, error)
	Create(maindb *model.Maindb) error
	Update(maindb *model.Maindb) error
	Delete(id string) error
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

const selectColumns = `id, created_at, category, subtype, data, available`

func (s *Store) GetAll() ([]*model.Maindb, error) {
	rows, err := s.db.Query("SELECT " + selectColumns + " FROM maindb")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*model.Maindb
	for rows.Next() {
		var m model.Maindb
		var createdAt sql.NullString
		var available sql.NullInt64
		if err := rows.Scan(&m.ID, &createdAt, &m.Category, &m.Subtype, &m.Data, &available); err != nil {
			return nil, err
		}
		if createdAt.Valid && createdAt.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, createdAt.String); err == nil {
				m.CreatedAt = t
			}
		}
		m.Available = available.Valid && available.Int64 != 0
		list = append(list, &m)
	}
	return list, rows.Err()
}

func (s *Store) GetByID(id string) (*model.Maindb, error) {
	var m model.Maindb
	var createdAt sql.NullString
	var available sql.NullInt64
	err := s.db.QueryRow("SELECT "+selectColumns+" FROM maindb WHERE id = ?", id).Scan(
		&m.ID, &createdAt, &m.Category, &m.Subtype, &m.Data, &available,
	)
	if err != nil {
		return nil, err
	}
	if createdAt.Valid && createdAt.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, createdAt.String); err == nil {
			m.CreatedAt = t
		}
	}
	m.Available = available.Valid && available.Int64 != 0
	return &m, nil
}

func (s *Store) GetByCategory(category string) ([]*model.Maindb, error) {
	rows, err := s.db.Query("SELECT "+selectColumns+" FROM maindb WHERE category = ?", category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*model.Maindb
	for rows.Next() {
		var m model.Maindb
		var createdAt sql.NullString
		var available sql.NullInt64
		if err := rows.Scan(&m.ID, &createdAt, &m.Category, &m.Subtype, &m.Data, &available); err != nil {
			return nil, err
		}
		if createdAt.Valid && createdAt.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, createdAt.String); err == nil {
				m.CreatedAt = t
			}
		}
		m.Available = available.Valid && available.Int64 != 0
		list = append(list, &m)
	}
	return list, rows.Err()
}

func (s *Store) Create(m *model.Maindb) error {
	m.ApplyDefaults()
	_, err := s.db.Exec(
		`INSERT INTO maindb (id, created_at, category, subtype, data, available) VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.CreatedAt.UTC().Format(time.RFC3339Nano), m.Category, m.Subtype, m.Data, boolToInt(m.Available),
	)
	return err
}

func (s *Store) Update(m *model.Maindb) error {
	res, err := s.db.Exec(
		`UPDATE maindb SET category = ?, subtype = ?, data = ?, available = ? WHERE id = ?`,
		m.Category, m.Subtype, m.Data, boolToInt(m.Available), m.ID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM maindb WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
