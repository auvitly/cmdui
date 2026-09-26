package auth

import (
	"errors"

	"cmdui/internal/domain"
	"cmdui/internal/repository/users"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid username or password")

type Service struct {
	repository users.Repository
}

func New(repository users.Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) EnsureConfigured(configured map[string]domain.User) error {
	for _, user := range configured {
		if err := s.repository.EnsureUser(user); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Authenticate(username, password string) (domain.User, error) {
	user, err := s.repository.GetUser(username)
	if err != nil || bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)) != nil {
		return domain.User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) Register(username, password string) error {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repository.SaveUser(domain.User{Username: username, Role: "user", PasswordHash: passwordHash})
}

func (s *Service) GetUser(username string) (domain.User, error) {
	return s.repository.GetUser(username)
}

func (s *Service) ListUsers(role string) ([]domain.User, error) {
	return s.repository.ListUsers(role)
}

func (s *Service) UpdateRole(username, role string) error {
	return s.repository.UpdateUserRole(username, role)
}

func (s *Service) DeleteUser(username, actorUsername string) error {
	return s.repository.DeleteUser(username, actorUsername)
}
