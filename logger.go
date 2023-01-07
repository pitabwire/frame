package frame

import (
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
	"io"
	"log"
	"os"
)

// Logger Option that helps with initialization of our internal logger
func Logger() Option {
	return func(s *Service) {
		logger := logrus.New()
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
		logger.SetReportCaller(true)
		log.SetOutput(io.Discard)
		logger.AddHook(&writer.Hook{
			Writer: os.Stderr,
			LogLevels: []logrus.Level{
				logrus.PanicLevel,
				logrus.FatalLevel,
				logrus.ErrorLevel,
				logrus.WarnLevel,
			},
		})
		logger.AddHook(&writer.Hook{
			Writer: os.Stdout,
			LogLevels: []logrus.Level{
				logrus.InfoLevel,
				logrus.DebugLevel,
			},
		})
		s.logger = logger
	}
}

func (s *Service) L() *logrus.Entry {
	return s.logger.WithField("service", s.Name())
}
