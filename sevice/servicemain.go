package service

import (
	"aflujo/model"
	"aflujo/store"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	store          store.Storer
	adminUser      string
	adminTokenHash string
}

func New(st store.Storer) *Service {
	return &Service{store: st}
}

var ErrAdminProtected = errors.New("admin client is protected")

type PermissionDeniedError struct {
	Advice string
}

func (e *PermissionDeniedError) Error() string {
	return e.Advice
}

func (s *Service) ConfigureAdmin(user, token string) {
	u := strings.TrimSpace(user)
	t := strings.TrimSpace(token)
	if u == "" {
		u = "admin"
	}
	if t == "" {
		t = "password"
	}
	s.adminUser = u
	s.adminTokenHash = store.TokenHash(t)
}

func (s *Service) EnsureAdminClient() error {
	if s.adminUser == "" || s.adminTokenHash == "" {
		s.ConfigureAdmin("admin", "password")
	}
	existing, err := s.store.GetClientByName(s.adminUser)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return s.store.CreateClient(&model.Client{
			ID:        uuid.NewString(),
			TokenHash: s.adminTokenHash,
			Name:      s.adminUser,
			CreatedAt: time.Now().UTC(),
		})
	}
	return s.store.UpdateClientByID(existing.ID, &s.adminUser, &s.adminTokenHash)
}

func (s *Service) IsAdminCredentials(user, token string) bool {
	return strings.TrimSpace(user) == s.adminUser && store.TokenHash(strings.TrimSpace(token)) == s.adminTokenHash
}

func (s *Service) GetAllFilteredForClient(clientID string, fromDate *time.Time, categories []string, available *bool, max *int, ord string) ([]*model.MaindbWithSync, error) {
	return s.store.GetFilteredForClient(clientID, fromDate, categories, available, max, ord)
}

func (s *Service) GetCategoryCounts(available *bool) ([]*model.CategoryCount, error) {
	return s.store.GetCategoryCounts(available)
}

func (s *Service) GetByID(id string) (*model.Maindb, error) {
	return s.store.GetByID(id)
}

func (s *Service) GetByIDForClient(id, clientID string) (*model.MaindbWithSync, error) {
	return s.store.GetByIDForClient(id, clientID)
}

func (s *Service) CreateMainWithSync(maindb *model.Maindb, clientID string) error {
	return s.store.CreateMainWithSync(maindb, clientID)
}

func (s *Service) UpdateMainAvailableFalseWithSync(id, clientID string) (*model.MaindbWithSync, error) {
	return s.store.UpdateMainAvailableFalseWithSync(id, clientID)
}

func (s *Service) GetClientByTokenHash(hash string) (*model.Client, error) {
	return s.store.GetClientByTokenHash(hash)
}

func (s *Service) GetClientByID(id string) (*model.Client, error) {
	return s.store.GetClientByID(id)
}

func containsCategory(list []string, category string) bool {
	for _, c := range list {
		if c == category {
			return true
		}
	}
	return false
}

func normalizeCategoryList(in []string) []string {
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

func permissionsAdvice(p *model.ClientPermissions, action, category string) string {
	return fmt.Sprintf(
		"Permiso denegado para %s en category %q. restricted=%v, max_create_categories=%d, read=%v, write=%v, create=%v.",
		action, category, p.Restricted, p.MaxCreateCategories, p.ReadCategories, p.WriteCategories, p.CreateCategories,
	)
}

func (s *Service) ValidateCategoryAccess(clientID, action, category string) error {
	p, err := s.store.GetClientPermissions(clientID)
	if err != nil {
		return err
	}
	if !p.Restricted {
		return nil
	}
	category = strings.TrimSpace(category)
	var allowed []string
	switch action {
	case "read":
		allowed = p.ReadCategories
	case "write":
		allowed = p.WriteCategories
	case "create":
		allowed = p.CreateCategories
	default:
		return fmt.Errorf("invalid permission action: %s", action)
	}
	if !containsCategory(allowed, category) {
		return &PermissionDeniedError{Advice: permissionsAdvice(p, action, category)}
	}
	return nil
}

func (s *Service) FilterReadableCategories(clientID string, categories []string) ([]string, error) {
	p, err := s.store.GetClientPermissions(clientID)
	if err != nil {
		return nil, err
	}
	if !p.Restricted {
		return categories, nil
	}

	if len(categories) == 0 {
		if len(p.ReadCategories) == 0 {
			return nil, &PermissionDeniedError{Advice: permissionsAdvice(p, "read", "(sin categorias permitidas)")}
		}
		return p.ReadCategories, nil
	}
	for _, c := range categories {
		if !containsCategory(p.ReadCategories, strings.TrimSpace(c)) {
			return nil, &PermissionDeniedError{Advice: permissionsAdvice(p, "read", strings.TrimSpace(c))}
		}
	}
	return categories, nil
}

func (s *Service) GetClientPermissions(clientID string) (*model.ClientPermissions, error) {
	return s.store.GetClientPermissions(clientID)
}

func (s *Service) UpsertClientPermissions(clientID string, body model.UpsertClientPermissionsBody) (*model.ClientPermissions, error) {
	body.ReadCategories = normalizeCategoryList(body.ReadCategories)
	body.WriteCategories = normalizeCategoryList(body.WriteCategories)
	body.CreateCategories = normalizeCategoryList(body.CreateCategories)

	if body.MaxCreateCategories < 0 {
		return nil, fmt.Errorf("max_create_categories must be >= 0")
	}
	if body.MaxCreateCategories > 0 && len(body.CreateCategories) > body.MaxCreateCategories {
		return nil, fmt.Errorf("create_categories exceeds max_create_categories")
	}
	p := &model.ClientPermissions{
		ClientID:            clientID,
		Restricted:          body.Restricted,
		MaxCreateCategories: body.MaxCreateCategories,
		ReadCategories:      body.ReadCategories,
		WriteCategories:     body.WriteCategories,
		CreateCategories:    body.CreateCategories,
	}
	if err := s.store.UpsertClientPermissions(p); err != nil {
		return nil, err
	}
	return s.store.GetClientPermissions(clientID)
}

// RegisterClient crea un cliente, genera token opaco (solo se devuelve en claro en la respuesta) y filas de sync para maindb existentes.
func (s *Service) RegisterClient(name string) (*model.ClientCreatedResponse, error) {
	plain := uuid.NewString()
	c := &model.Client{
		ID:        uuid.NewString(),
		TokenHash: store.TokenHash(plain),
		Name:      strings.TrimSpace(name),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateClient(c); err != nil {
		return nil, err
	}
	return &model.ClientCreatedResponse{
		ID:        c.ID,
		Name:      c.Name,
		Token:     plain,
		CreatedAt: c.CreatedAt,
	}, nil
}

func (s *Service) ListClients() ([]*model.Client, error) {
	return s.store.ListClients()
}

func (s *Service) UpdateClient(id string, name *string, rotateToken bool) (*model.ClientUpdatedResponse, error) {
	c, err := s.store.GetClientByID(id)
	if err != nil {
		return nil, err
	}
	if c.Name == s.adminUser {
		return nil, ErrAdminProtected
	}

	var updName *string
	if name != nil {
		n := strings.TrimSpace(*name)
		if n == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		updName = &n
	}

	var (
		updTokenHash *string
		plainToken   *string
	)
	if rotateToken {
		t := uuid.NewString()
		h := store.TokenHash(t)
		updTokenHash = &h
		plainToken = &t
	}

	if updName == nil && updTokenHash == nil {
		return nil, fmt.Errorf("no changes requested")
	}
	if err := s.store.UpdateClientByID(id, updName, updTokenHash); err != nil {
		return nil, err
	}
	updated, err := s.store.GetClientByID(id)
	if err != nil {
		return nil, err
	}
	return &model.ClientUpdatedResponse{
		ID:        updated.ID,
		Name:      updated.Name,
		CreatedAt: updated.CreatedAt,
		Token:     plainToken,
	}, nil
}

func (s *Service) DeleteClient(id string) error {
	c, err := s.store.GetClientByID(id)
	if err != nil {
		return err
	}
	if c.Name == s.adminUser {
		return ErrAdminProtected
	}
	return s.store.DeleteClientByID(id)
}
