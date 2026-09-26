package commands

import "github.com/auvitly/cmdui.git/internal/domain"

type Repository interface {
	SaveCommand(domain.Command) (int64, error)
	GetCommand(id int64) (domain.Command, error)
	ListCommands(operatorOnly bool) ([]domain.Command, error)
	SetOperatorAllowed(id int64, allowed bool) error
	SetLastApplied(id int64, username string) error
	DeleteCommand(id int64) error
	SaveCustomIcon(icon domain.CustomIcon) (int64, error)
	ListCustomIcons() ([]domain.CustomIcon, error)
	GetCustomIcon(id int64) (domain.CustomIcon, error)
	DeleteCustomIcon(id int64) error
}
