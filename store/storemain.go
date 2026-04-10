package store

import (
	"aflujo/model"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Storer define el contrato de persistencia para Maindb, clientes y sync.
type Storer interface {
	GetFilteredForClient(clientID string, fromDate *time.Time, categories []string, available *bool, max *int, ord string) ([]*model.MaindbWithSync, error)
	GetCategoryCounts(available *bool) ([]*model.CategoryCount, error)
	GetClientPermissions(clientID string) (*model.ClientPermissions, error)
	UpsertClientPermissions(p *model.ClientPermissions) error
	GetByID(id string) (*model.Maindb, error)
	GetByIDForClient(id, clientID string) (*model.MaindbWithSync, error)
	CreateMainWithSync(m *model.Maindb, authorClientID string) error
	UpdateMainAvailableFalseWithSync(id, callerClientID string) (*model.MaindbWithSync, error)
	GetClientByTokenHash(hash string) (*model.Client, error)
	GetClientByID(id string) (*model.Client, error)
	GetClientByName(name string) (*model.Client, error)
	ListClients() ([]*model.Client, error)
	CreateClient(c *model.Client) error
	UpdateClientByID(id string, name, tokenHash *string) error
	DeleteClientByID(id string) error
	BackfillMissingSyncRows() error
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

const selectMainColumns = `m.id, m.created_at, m.category, m.subtype, m.data, m.available`

func scanMaindbRowSingle(row *sql.Row) (*model.Maindb, error) {
	var m model.Maindb
	var createdAt sql.NullString
	var available sql.NullInt64
	if err := row.Scan(&m.ID, &createdAt, &m.Category, &m.Subtype, &m.Data, &available); err != nil {
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

func (s *Store) GetByID(id string) (*model.Maindb, error) {
	row := s.db.QueryRow("SELECT id, created_at, category, subtype, data, available FROM maindb WHERE id = ?", id)
	return scanMaindbRowSingle(row)
}

func (s *Store) GetByIDForClient(id, clientID string) (*model.MaindbWithSync, error) {
	q := `
SELECT ` + selectMainColumns + `,
	COALESCE(s.synced, 0) AS synced
FROM maindb m
LEFT JOIN maindb_client_sync s ON s.maindb_id = m.id AND s.client_id = ?
WHERE m.id = ?`
	row := s.db.QueryRow(q, clientID, id)
	var m model.Maindb
	var createdAt sql.NullString
	var available sql.NullInt64
	var syncedInt sql.NullInt64
	if err := row.Scan(&m.ID, &createdAt, &m.Category, &m.Subtype, &m.Data, &available, &syncedInt); err != nil {
		return nil, err
	}
	if createdAt.Valid && createdAt.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, createdAt.String); err == nil {
			m.CreatedAt = t
		}
	}
	m.Available = available.Valid && available.Int64 != 0
	synced := syncedInt.Valid && syncedInt.Int64 != 0
	return &model.MaindbWithSync{Maindb: m, Synced: synced}, nil
}

func (s *Store) GetFilteredForClient(clientID string, fromDate *time.Time, categories []string, available *bool, max *int, ord string) ([]*model.MaindbWithSync, error) {
	var (
		sb   strings.Builder
		args []any
	)

	sb.WriteString(`SELECT ` + selectMainColumns + `, COALESCE(s.synced, 0) AS synced
FROM maindb m
LEFT JOIN maindb_client_sync s ON s.maindb_id = m.id AND s.client_id = ?
WHERE 1=1`)
	args = append(args, clientID)

	if fromDate != nil {
		sb.WriteString(" AND m.created_at >= ?")
		args = append(args, fromDate.UTC().Format(time.RFC3339Nano))
	}

	if available != nil {
		sb.WriteString(" AND m.available = ?")
		args = append(args, boolToInt(*available))
	}

	if len(categories) > 0 {
		sb.WriteString(" AND m.category IN (")
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
	sb.WriteString(" ORDER BY m.created_at ")
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

	var list []*model.MaindbWithSync
	for rows.Next() {
		var m model.Maindb
		var createdAt sql.NullString
		var avail sql.NullInt64
		var syncedInt sql.NullInt64
		if err := rows.Scan(&m.ID, &createdAt, &m.Category, &m.Subtype, &m.Data, &avail, &syncedInt); err != nil {
			return nil, err
		}
		if createdAt.Valid && createdAt.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, createdAt.String); err == nil {
				m.CreatedAt = t
			}
		}
		m.Available = avail.Valid && avail.Int64 != 0
		synced := syncedInt.Valid && syncedInt.Int64 != 0
		list = append(list, &model.MaindbWithSync{Maindb: m, Synced: synced})
	}
	return list, rows.Err()
}

func (s *Store) GetCategoryCounts(available *bool) ([]*model.CategoryCount, error) {
	q := `SELECT category, COUNT(*) AS total FROM maindb`
	args := make([]any, 0, 1)
	if available != nil {
		q += ` WHERE available = ?`
		args = append(args, boolToInt(*available))
	}
	q += ` GROUP BY category ORDER BY total DESC, category ASC`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*model.CategoryCount, 0)
	for rows.Next() {
		var row model.CategoryCount
		if err := rows.Scan(&row.Category, &row.Total); err != nil {
			return nil, err
		}
		out = append(out, &row)
	}
	return out, rows.Err()
}

func listClientIDsTx(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM clients`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func upsertSyncForMaindbTx(tx *sql.Tx, maindbID, authorClientID string) error {
	ids, err := listClientIDsTx(tx)
	if err != nil {
		return err
	}
	for _, cid := range ids {
		v := 0
		if cid == authorClientID {
			v = 1
		}
		if _, err := tx.Exec(
			`INSERT OR REPLACE INTO maindb_client_sync (maindb_id, client_id, synced) VALUES (?, ?, ?)`,
			maindbID, cid, v,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateMainWithSync(m *model.Maindb, authorClientID string) error {
	m.ApplyDefaults()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`INSERT INTO maindb (id, created_at, category, subtype, data, available) VALUES (?, ?, ?, ?, ?, ?)`,
		m.ID, m.CreatedAt.UTC().Format(time.RFC3339Nano), m.Category, m.Subtype, m.Data, boolToInt(m.Available),
	); err != nil {
		return err
	}
	if err := upsertSyncForMaindbTx(tx, m.ID, authorClientID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateMainAvailableFalseWithSync(id, callerClientID string) (*model.MaindbWithSync, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRow(`SELECT id, created_at, category, subtype, data, available FROM maindb WHERE id = ?`, id)
	m, err := scanMaindbRowSingle(row)
	if err != nil {
		return nil, err
	}
	if !m.Available {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.GetByIDForClient(id, callerClientID)
	}

	res, err := tx.Exec(`UPDATE maindb SET available = ? WHERE id = ?`, 0, id)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	m.Available = false
	if err := upsertSyncForMaindbTx(tx, id, callerClientID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetByIDForClient(id, callerClientID)
}

func (s *Store) GetClientByTokenHash(hash string) (*model.Client, error) {
	return s.getClientByWhere("token_hash = ?", hash)
}

func (s *Store) GetClientByID(id string) (*model.Client, error) {
	return s.getClientByWhere("id = ?", id)
}

func (s *Store) GetClientByName(name string) (*model.Client, error) {
	return s.getClientByWhere("name = ?", name)
}

func (s *Store) getClientByWhere(where string, arg any) (*model.Client, error) {
	var c model.Client
	var createdAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, token_hash, name, created_at FROM clients WHERE `+where,
		arg,
	).Scan(&c.ID, &c.TokenHash, &c.Name, &createdAt)
	if err != nil {
		return nil, err
	}
	if createdAt.Valid && createdAt.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, createdAt.String); err == nil {
			c.CreatedAt = t
		}
	}
	return &c, nil
}

func (s *Store) ListClients() ([]*model.Client, error) {
	rows, err := s.db.Query(`SELECT id, token_hash, name, created_at FROM clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*model.Client, 0)
	for rows.Next() {
		var c model.Client
		var createdAt sql.NullString
		if err := rows.Scan(&c.ID, &c.TokenHash, &c.Name, &createdAt); err != nil {
			return nil, err
		}
		if createdAt.Valid && createdAt.String != "" {
			if t, err := time.Parse(time.RFC3339Nano, createdAt.String); err == nil {
				c.CreatedAt = t
			}
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (s *Store) CreateClient(c *model.Client) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`INSERT INTO clients (id, token_hash, name, created_at) VALUES (?, ?, ?, ?)`,
		c.ID, c.TokenHash, c.Name, c.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO maindb_client_sync (maindb_id, client_id, synced)
		SELECT id, ?, 0 FROM maindb`,
		c.ID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateClientByID(id string, name, tokenHash *string) error {
	if name == nil && tokenHash == nil {
		return fmt.Errorf("nothing to update")
	}
	var (
		sets []string
		args []any
	)
	if name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *name)
	}
	if tokenHash != nil {
		sets = append(sets, "token_hash = ?")
		args = append(args, *tokenHash)
	}
	args = append(args, id)

	q := `UPDATE clients SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
	res, err := s.db.Exec(q, args...)
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

func (s *Store) DeleteClientByID(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM maindb_client_sync WHERE client_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM clients WHERE id = ?`, id)
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
	return tx.Commit()
}

func (s *Store) BackfillMissingSyncRows() error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO maindb_client_sync (maindb_id, client_id, synced)
		SELECT m.id, c.id, 0 FROM maindb m CROSS JOIN clients c
	`)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func normalizeCategories(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		c := strings.TrimSpace(raw)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func loadPermissionCategoriesTx(tx *sql.Tx, clientID, action string) ([]string, error) {
	rows, err := tx.Query(
		`SELECT category FROM client_category_permissions WHERE client_id = ? AND action = ? ORDER BY category ASC`,
		clientID, action,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetClientPermissions(clientID string) (*model.ClientPermissions, error) {
	p := &model.ClientPermissions{
		ClientID:            clientID,
		Restricted:          false,
		MaxCreateCategories: 0,
		ReadCategories:      nil,
		WriteCategories:     nil,
		CreateCategories:    nil,
	}

	row := s.db.QueryRow(
		`SELECT restricted, max_create_categories FROM client_permissions WHERE client_id = ?`,
		clientID,
	)
	var restrictedInt sql.NullInt64
	var maxCreate sql.NullInt64
	err := row.Scan(&restrictedInt, &maxCreate)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, nil
		}
		return nil, err
	}
	p.Restricted = restrictedInt.Valid && restrictedInt.Int64 != 0
	if maxCreate.Valid && maxCreate.Int64 > 0 {
		p.MaxCreateCategories = int(maxCreate.Int64)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if p.ReadCategories, err = loadPermissionCategoriesTx(tx, clientID, "read"); err != nil {
		return nil, err
	}
	if p.WriteCategories, err = loadPermissionCategoriesTx(tx, clientID, "write"); err != nil {
		return nil, err
	}
	if p.CreateCategories, err = loadPermissionCategoriesTx(tx, clientID, "create"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}

func insertPermissionCategoriesTx(tx *sql.Tx, clientID, action string, categories []string) error {
	for _, c := range normalizeCategories(categories) {
		if _, err := tx.Exec(
			`INSERT INTO client_category_permissions (client_id, action, category) VALUES (?, ?, ?)`,
			clientID, action, c,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertClientPermissions(p *model.ClientPermissions) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`INSERT INTO client_permissions (client_id, restricted, max_create_categories)
		 VALUES (?, ?, ?)
		 ON CONFLICT(client_id) DO UPDATE SET restricted = excluded.restricted, max_create_categories = excluded.max_create_categories`,
		p.ClientID, boolToInt(p.Restricted), p.MaxCreateCategories,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM client_category_permissions WHERE client_id = ?`, p.ClientID); err != nil {
		return err
	}
	if err := insertPermissionCategoriesTx(tx, p.ClientID, "read", p.ReadCategories); err != nil {
		return err
	}
	if err := insertPermissionCategoriesTx(tx, p.ClientID, "write", p.WriteCategories); err != nil {
		return err
	}
	if err := insertPermissionCategoriesTx(tx, p.ClientID, "create", p.CreateCategories); err != nil {
		return err
	}
	return tx.Commit()
}
