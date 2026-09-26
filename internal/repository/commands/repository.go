package commands

import "cmdui/internal/domain"

type Repository interface {
	SaveCommand(domain.Command) (int64, error)
	GetCommand(id int64) (domain.Command, error)
	ListCommands(operatorOnly bool) ([]domain.Command, error)
	SetOperatorAllowed(id int64, allowed bool) error
	SetLastApplied(id int64, username string) error
	DeleteCommand(id int64) error
}
