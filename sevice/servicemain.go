package service

import (
	"aflujo/model"
	"aflujo/store"
	"time"
)

type Service struct {
	store store.Storer
}

func New(st store.Storer) *Service {
	return &Service{store: st}
}

func (s *Service) GetAll() ([]*model.Maindb, error) {
	return s.store.GetAll()
}

func (s *Service) GetAllFiltered(fromDate *time.Time, categories []string, available *bool, max *int, ord string) ([]*model.Maindb, error) {
	return s.store.GetFiltered(fromDate, categories, available, max, ord)
}

func (s *Service) GetByID(id string) (*model.Maindb, error) {
	return s.store.GetByID(id)
}

func (s *Service) Create(maindb *model.Maindb) error {
	return s.store.Create(maindb)
}

func (s *Service) Update(maindb *model.Maindb) error {
	return s.store.Update(maindb)
}