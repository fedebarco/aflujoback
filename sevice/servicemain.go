package service

import (
	"aflujo/model"
	"aflujo/store"
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

func (s *Service) GetByID(id string) (*model.Maindb, error) {
	return s.store.GetByID(id)
}

func (s *Service) GetByCategory(category string) ([]*model.Maindb, error) {
	return s.store.GetByCategory(category)
}

func (s *Service) Create(maindb *model.Maindb) error {
	return s.store.Create(maindb)
}

func (s *Service) Update(maindb *model.Maindb) error {
	return s.store.Update(maindb)
}

func (s *Service) Delete(id string) error {
	return s.store.Delete(id)
}