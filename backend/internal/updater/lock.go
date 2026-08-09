package updater

import "errors"

var (
	ErrOperationActive              = errors.New("another update operation is active")
	ErrOperatorInterventionRequired = errors.New("updater is locked pending operator intervention")
)

type operationLock interface {
	Release() error
}
