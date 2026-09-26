package users

import "github.com/auvitly/cmdui.git/internal/domain"

type Repository interface {
	SaveUser(domain.User) error
	EnsureUser(domain.User) error
	GetUser(string) (domain.User, error)
	ListUsers(role string) ([]domain.User, error)
	UpdateUserRole(username, role string) error
	DeleteUser(username, actorUsername string) error
}
