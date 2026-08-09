package updater

import "context"

type Service struct {
	controller *Controller
}

func NewService(controller *Controller) *Service {
	return &Service{controller: controller}
}

func (s *Service) RequestUpdate(releaseID string) (Operation, error) {
	return s.controller.Start(releaseID)
}

func (s *Service) Check(ctx context.Context) (CheckResult, error) {
	return s.controller.Check(ctx)
}

func (s *Service) Current(ctx context.Context) (InstalledRelease, error) {
	return s.controller.Current(ctx)
}

func (s *Service) Operation(id string) (Operation, error) {
	return s.controller.Get(id)
}

func (s *Service) OperationEvents(id string) ([]Event, error) {
	return s.controller.Events(id)
}

func (s *Service) ActiveOperation() (*Operation, error) {
	return s.controller.Active()
}

func (s *Service) Recover() (*Operation, error) {
	return s.controller.Recover()
}

func (s *Service) Wait(ctx context.Context, id string) (Operation, error) {
	return s.controller.Wait(ctx, id)
}
