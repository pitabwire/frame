package workloadapi_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/pitabwire/frame/config"
	frameworkloadapi "github.com/pitabwire/frame/security/workloadapi"
)

type SetupSuite struct {
	suite.Suite
}

func TestSetupSuite(t *testing.T) {
	suite.Run(t, new(SetupSuite))
}

func (s *SetupSuite) TestSetupRequiresTrustedDomain() {
	api := frameworkloadapi.NewWorkloadAPI(&config.ConfigurationDefault{})

	tlsConfig, err := api.Setup(context.Background())

	s.Nil(tlsConfig)
	s.Require().Error(err)
	s.Contains(err.Error(), "no trust domain")
}

func (s *SetupSuite) TestSetupRejectsInvalidTrustedDomainWithoutPanicking() {
	api := frameworkloadapi.NewWorkloadAPI(&config.ConfigurationDefault{
		WorkloadAPITrustedDomain: "not a valid trust domain",
	})

	s.NotPanics(func() {
		tlsConfig, err := api.Setup(context.Background())
		s.Nil(tlsConfig)
		s.Require().Error(err)
	})
}
