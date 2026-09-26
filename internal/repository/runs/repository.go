package runs

import "cmdui/internal/domain"

type Repository interface {
	SaveRun(domain.Run) (int64, error)
	UpdateRun(domain.Run) error
	GetRun(id int64) (domain.Run, error)
	ListRuns(filter domain.RunFilter, limit, offset int) ([]domain.Run, error)
	CountRuns(filter domain.RunFilter) (int, error)
}
