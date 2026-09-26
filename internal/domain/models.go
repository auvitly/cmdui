package domain

import (
	"errors"
	"time"
)

type User struct {
	Username     string
	Role         string
	PasswordHash []byte
}

type Label struct {
	Key   string
	Value string
	Color string
}

type CustomIcon struct {
	ID        int64
	Name      string
	SVG       string
	CreatedAt time.Time
}

type Command struct {
	ID                int64
	Name              string
	Description       string
	Icon              string
	Program           string
	Args              []string
	Script            string
	Labels            []Label
	TimeoutSeconds    int
	AccessOperators   bool
	AccessAllUsers    bool
	SpecificUsers     []string
	SpecificOperators []string
	OperatorsCanRun   bool
	OperatorAllowed   bool
	LastAppliedBy     string
	LastAppliedAt     time.Time
	UpdatedAt         time.Time
}

type Run struct {
	ID            int64
	Username      string
	CommandName   string
	CommandScript string
	Program       string
	Args          []string
	Status        string
	ExitCode      int
	StartedAt     time.Time
	Duration      time.Duration
	Stdout        string
	Stderr        string
}

type RunFilter struct {
	Username      string
	UsernameExact bool
	Command       string
	Status        string
	From          string
	To            string
}

var (
	ErrLastAdmin         = errors.New("cannot remove the last admin role")
	ErrCannotDeleteAdmin = errors.New("only the superadmin can delete an admin")
	ErrCannotDeleteSelf  = errors.New("cannot delete the active account")
)
