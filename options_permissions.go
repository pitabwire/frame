package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/pitabwire/util"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// PermissionManifest is the payload sent to the authorization service to
// register a service's permission namespace, available permissions, and
// role-to-permission mappings.
type PermissionManifest struct {
	Namespace    string              `json:"namespace"`
	Permissions  []string            `json:"permissions"`
	RoleBindings map[string][]string `json:"role_bindings"`
	RegisteredAt time.Time           `json:"registered_at"`
}

// manifestRegistrationURLEnvVar is the environment variable that provides
// the full URL for permission manifest registration.
const manifestRegistrationURLEnvVar = "PERMISSIONS_REGISTRATION_URL"

// ManifestBuilder builds a PermissionManifest from a proto service descriptor.
// Frame uses this interface to avoid importing the common/permissions package
// directly, keeping the dependency flow clean.
type ManifestBuilder func(sd protoreflect.ServiceDescriptor) PermissionManifest

// WithPermissionRegistration registers a service's permission manifest with the
// authorization service at startup. The manifest is extracted from the proto
// service descriptor's service_permissions annotation.
//
// Registration is asynchronous and best-effort — it does not block service
// startup or cause failures if the authorization service is unavailable.
// The authorization service URL is read from the PERMISSIONS_REGISTRATION_URL
// environment variable.
//
// Usage:
//
//	sd := profilepb.File_profile_v1_profile_proto.Services().ByName("ProfileService")
//	frame.WithPermissionRegistration(sd, permissions.BuildManifest)
func WithPermissionRegistration(sd protoreflect.ServiceDescriptor, builder ManifestBuilder) Option {
	return func(_ context.Context, s *Service) {
		registrationURL := os.Getenv(manifestRegistrationURLEnvVar)
		if registrationURL == "" {
			return
		}

		s.AddPreStartMethod(func(ctx context.Context, _ *Service) {
			manifest := builder(sd)
			if manifest.Namespace == "" {
				return
			}

			go publishManifest(ctx, registrationURL, manifest)
		})
	}
}

func publishManifest(ctx context.Context, url string, manifest PermissionManifest) {
	logger := util.Log(ctx).WithField("namespace", manifest.Namespace)

	body, err := json.Marshal(manifest)
	if err != nil {
		logger.WithError(err).Warn("failed to marshal permission manifest")
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		logger.WithError(err).Warn("failed to create permission manifest request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	const httpTimeout = 10 * time.Second
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Warn("failed to publish permission manifest")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		logger.WithField("status", resp.StatusCode).
			Warn(fmt.Sprintf("permission manifest registration returned %d", resp.StatusCode))
		return
	}

	logger.Debug("permission manifest registered")
}
