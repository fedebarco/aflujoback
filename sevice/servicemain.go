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
