package frame

import (
	"github.com/sirupsen/logrus"
)

// Logger Option that helps with initialization of our internal logger
func Logger() Option {
	return func(s *Service) {

		logger := logrus.New()
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
		logger.SetReportCaller(true)
		logger.WithField("service", s.Name())
		s.logger = logger
	}
}

func (s *Service) L() *logrus.Logger {
	return s.logger
}
