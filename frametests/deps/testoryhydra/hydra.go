package testoryhydra

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// OryHydraImage is the Ory Hydra Image.
	OryHydraImage = "oryd/hydra:v2.3.0"

	HydraConfiguration = `
## ORY Hydra Configuration
#
log:
  level: warn
  leak_sensitive_values: true

serve:
  admin:
    host: 0.0.0.0
  public:
    host: 0.0.0.0
  cookies:
    same_site_mode: Lax

urls:
  self:
    issuer: http://127.0.0.1:4444
    public: http://127.0.0.1:4444
  consent: http://127.0.0.1:3000/consent
  login: http://127.0.0.1:3000/login
  logout: http://127.0.0.1:3000/logout
  
oauth2:
  session:
    encrypt_at_rest: true
  exclude_not_before_claim: true
  allowed_top_level_claims:
    - contact_id
    - tenant_id
    - partition_id
    - access_id
    - roles
    - username
    - service_name
  mirror_top_level_claims: false

secrets:
  system:
    - NzItNDQ5ZS04MTBkLWM0ODBjNjhjZ

oidc:
  dynamic_client_registration:
    default_scope:
      - openid
      - profile
      - contact
      - offline_access
    enabled: false
  subject_identifiers:
    supported_types:
      - public

strategies:
  access_token: jwt
  scope: wildcard

`
)

type dependancy struct {
	*definition.DefaultImpl
	configuration string
}

func New() definition.TestResource {
	return NewWithOpts(HydraConfiguration)
}

func NewWithOpts(configuration string, containerOpts ...definition.ContainerOption) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      OryHydraImage,
		Ports:          []string{"4445/tcp", "4444/tcp"},
		NetworkAliases: []string{"hydra", "auth-hydra"},
	}
	opts.Setup(containerOpts...)

	return &dependancy{
		DefaultImpl:   definition.NewDefaultImpl(opts, "http"),
		configuration: configuration,
	}
}

func (d *dependancy) migrateContainer(
	ctx context.Context,
	ntwk *testcontainers.DockerNetwork,
	databaseURL string,
) error {
	containerRequest := testcontainers.ContainerRequest{
		Image: d.Name(),
		Cmd:   []string{"migrate", "sql", "up", "--read-from-env", "--yes", "--config", "/etc/config/hydra.yml"},
		Env: map[string]string{
			"LOG_LEVEL": "debug",
			"DSN":       databaseURL,
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(d.configuration),
				ContainerFilePath: "/etc/config/hydra.yml",
				FileMode:          definition.ContainerFileMode,
			},
		},
		WaitingFor: wait.ForExit(),
	}

	d.Configure(ctx, ntwk, &containerRequest)

	hydraContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: containerRequest,
		Started:          true,
	})
	if err != nil {
		return err
	}

	err = hydraContainer.Terminate(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (d *dependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	if len(d.Opts().Dependencies) == 0 || !d.Opts().Dependencies[0].GetDS(ctx).IsDB() {
		return errors.New("no ByIsDatabase dependencies was supplied")
	}

	hydraDatabase, _, err := testpostgres.CreateDatabase(ctx, d.Opts().Dependencies[0].GetInternalDS(ctx), "hydra")
	if err != nil {
		return err
	}

	databaseURL := hydraDatabase.String()
	err = d.migrateContainer(ctx, ntwk, databaseURL)
	if err != nil {
		return err
	}

	containerRequest := testcontainers.ContainerRequest{
		Image: d.Name(),
		Cmd:   []string{"serve", "all", "--config", "/etc/config/hydra.yml", "--dev"},
		Env: d.Opts().Env(map[string]string{
			"LOG_LEVEL":                 "debug",
			"LOG_LEAK_SENSITIVE_VALUES": "true",
			"DSN":                       databaseURL,
		}),
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(d.configuration),
				ContainerFilePath: "/etc/config/hydra.yml",
				FileMode:          definition.ContainerFileMode,
			},
		},
		WaitingFor: wait.ForHTTP("/health/ready").WithPort(d.DefaultPort),
	}

	d.Configure(ctx, ntwk, &containerRequest)

	hydraContainer, err := testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: containerRequest,
			Started:          true,
		})

	if err != nil {
		return fmt.Errorf("failed to start hydraContainer: %w", err)
	}

	d.SetContainer(hydraContainer)
	return nil
}
