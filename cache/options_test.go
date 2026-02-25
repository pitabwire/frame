package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type OptionsSuite struct {
	suite.Suite
}

func TestOptionsSuite(t *testing.T) {
	suite.Run(t, new(OptionsSuite))
}

func (s *OptionsSuite) TestOptionsAppliersTable() {
	opts := &Options{}

	apply := []struct {
		name string
		fn   Option
	}{
		{name: "dsn", fn: WithDSN("redis://127.0.0.1:6379")},
		{name: "creds", fn: WithCredsFile("/tmp/creds")},
		{name: "name", fn: WithName("bucket")},
		{name: "max_age", fn: WithMaxAge(5 * time.Minute)},
	}

	for _, tc := range apply {
		s.Run(tc.name, func() {
			tc.fn(opts)
		})
	}

	s.Equal("redis://127.0.0.1:6379", opts.DSN.String())
	s.Equal("/tmp/creds", opts.CredsFile)
	s.Equal("bucket", opts.Name)
	s.Equal(5*time.Minute, opts.MaxAge)
}
