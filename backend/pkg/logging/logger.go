package logging

import "go.uber.org/zap"

func New(env string) *zap.Logger {
	if env == "local" || env == "development" || env == "dev" {
		logger, err := zap.NewDevelopment()
		if err == nil {
			return logger
		}
	}

	logger, err := zap.NewProduction()
	if err != nil {
		return zap.NewNop()
	}
	return logger
}
