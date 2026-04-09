package store

import (
	"database/sql"
	"time"
	"aflujo/model"
	"fmt"
	"strings"
)

// Storer define el contrato de persistencia para Maindb.
type Storer interface {
	GetAll() ([]*model.Maindb, error)
	GetFiltered(fromDate *time.Time, categories []string, available *bool, max *int, ord string) ([]*model.Maindb, error)
	GetByID(id string) (*model.Maindb, error)
	Create(maindb *model.Maindb) error
	Update(maindb *model.Maindb) error
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

func (s *Store) GetFiltered(fromDate *time.Time, categories []string, available *bool, max *int, ord string) ([]*model.Maindb, error) {
	var (
		sb   strings.Builder
		args []any
	)

	sb.WriteString("SELECT ")
	sb.WriteString(selectColumns)
	sb.WriteString(" FROM maindb WHERE 1=1")

	if fromDate != nil {
		sb.WriteString(" AND created_at >= ?")
		args = append(args, fromDate.UTC().Format(time.RFC3339Nano))
	}

	if available != nil {
		sb.WriteString(" AND available = ?")
		args = append(args, boolToInt(*available))
	}

	if len(categories) > 0 {
		sb.WriteString(" AND category IN (")
		for i := range categories {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
			args = append(args, categories[i])
		}
		sb.WriteString(")")
	}

	if ord != "asc" && ord != "desc" && ord != "" {
		return nil, fmt.Errorf("invalid ord: %q", ord)
	}
	if ord == "" {
		ord = "desc"
	}
	sb.WriteString(" ORDER BY created_at ")
	sb.WriteString(strings.ToUpper(ord))

	if max != nil {
		sb.WriteString(" LIMIT ?")
		args = append(args, *max)
	}

	rows, err := s.db.Query(sb.String(), args...)
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



func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
