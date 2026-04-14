package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pitabwire/util"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/pitabwire/frame/config"
)

// ManifestRegistrationURLEnvVar is the environment variable that provides
// the full URL for permission manifest registration. Services set this to
// point at the tenancy service's internal registration endpoint.
const ManifestRegistrationURLEnvVar = "PERMISSIONS_REGISTRATION_URL"

// servicePermissionsExtNumber is the proto extension field number for
// ServicePermissions on google.protobuf.ServiceOptions, as defined in
// common/v1/permissions.proto.
const servicePermissionsExtNumber protoreflect.FieldNumber = 50000

// Proto field numbers within ServicePermissions and RoleBinding messages.
const (
	fieldNamespace    protoreflect.FieldNumber = 1
	fieldPermissions  protoreflect.FieldNumber = 2
	fieldRoleBindings protoreflect.FieldNumber = 3
)

// initialBackoff is the initial retry delay for permission manifest registration.
const initialBackoff = 2 * time.Second

// maxBackoff is the maximum retry delay for permission manifest registration.
const maxBackoff = 30 * time.Second

// backoffMultiplier is the factor by which the backoff delay increases on each retry.
const backoffMultiplier = 2

// ManifestRegistrationPath is the default HTTP path for the internal
// permission manifest registration endpoint on the tenancy service.
const ManifestRegistrationPath = "/_internal/register/permissions"

// WithPermissionRegistration registers a service's permission manifest with
// the tenancy service during the migration phase. The manifest (namespace,
// permissions, role bindings) is extracted from the proto service descriptor's
// service_permissions annotation using proto reflection.
//
// Registration only runs when DO_MIGRATION=true or the "migrate" argument is
// passed — the same condition that triggers database migrations. It blocks
// with exponential backoff until the tenancy service accepts the manifest,
// ensuring the service's authorization namespace is configured before any
// requests are served.
//
// The tenancy service URL is read from PERMISSIONS_REGISTRATION_URL.
//
// Usage:
//
//	sd := profilepb.File_profile_v1_profile_proto.Services().ByName("ProfileService")
//	frame.WithPermissionRegistration(sd)
func WithPermissionRegistration(sd protoreflect.ServiceDescriptor) Option {
	return func(_ context.Context, s *Service) {
		registrationURL := os.Getenv(ManifestRegistrationURLEnvVar)
		if registrationURL == "" {
			return
		}

		cfg := s.Config()
		migrateCfg, ok := cfg.(config.ConfigurationDatabase)
		if !ok || !migrateCfg.DoDatabaseMigrate() {
			return
		}

		s.AddPreStartMethod(func(ctx context.Context, _ *Service) {
			manifest := buildManifestFromDescriptor(sd)
			if manifest == nil {
				return
			}
			publishManifestUntilSuccess(ctx, registrationURL, manifest)
		})
	}
}

// buildManifestFromDescriptor extracts a permission manifest from a proto
// service descriptor using pure proto reflection. It reads the
// service_permissions extension (field 50000) to get namespace, permissions,
// and role bindings without importing the typed proto package.
func buildManifestFromDescriptor(sd protoreflect.ServiceDescriptor) map[string]any {
	opts := sd.Options()
	if opts == nil {
		return nil
	}

	msg := opts.ProtoReflect()
	var manifest map[string]any

	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Number() != servicePermissionsExtNumber || !fd.IsExtension() {
			return true
		}

		extMsg := v.Message()
		manifest = extractManifestFields(extMsg)

		return false // found our extension, stop iterating
	})

	return manifest
}

// extractManifestFields reads the namespace, permissions, and role_bindings
// from a ServicePermissions proto message using field-number-based reflection.
func extractManifestFields(extMsg protoreflect.Message) map[string]any {
	desc := extMsg.Descriptor()
	manifest := map[string]any{
		"registered_at": time.Now().UTC(),
	}

	// Field 1: namespace (string)
	if nsField := desc.Fields().ByNumber(fieldNamespace); nsField != nil {
		ns := extMsg.Get(nsField).String()
		if ns == "" {
			return nil
		}
		manifest["namespace"] = ns
	}

	// Field 2: permissions (repeated string)
	if permField := desc.Fields().ByNumber(fieldPermissions); permField != nil {
		list := extMsg.Get(permField).List()
		perms := make([]string, list.Len())
		for i := range list.Len() {
			perms[i] = list.Get(i).String()
		}
		manifest["permissions"] = perms
	}

	// Field 3: role_bindings (repeated RoleBinding message)
	if rbField := desc.Fields().ByNumber(fieldRoleBindings); rbField != nil {
		manifest["role_bindings"] = extractRoleBindings(extMsg.Get(rbField).List())
	}

	return manifest
}

// extractRoleBindings reads role binding entries from a repeated RoleBinding field.
func extractRoleBindings(list protoreflect.List) map[string][]string {
	bindings := make(map[string][]string, list.Len())
	for i := range list.Len() {
		rbMsg := list.Get(i).Message()
		roleEnum := rbMsg.Get(rbMsg.Descriptor().Fields().ByNumber(fieldNamespace)).Enum()
		permsList := rbMsg.Get(rbMsg.Descriptor().Fields().ByNumber(fieldPermissions)).List()
		perms := make([]string, permsList.Len())
		for j := range permsList.Len() {
			perms[j] = permsList.Get(j).String()
		}
		roleName := standardRoleName(int32(roleEnum))
		if roleName != "" {
			bindings[roleName] = perms
		}
	}
	return bindings
}

// standardRoleName converts a StandardRole enum value to its lowercase name.
// Matches the enum in common/v1/permissions.proto:
// 0=UNSPECIFIED, 1=OWNER, 2=ADMIN, 3=OPERATOR, 4=VIEWER, 5=MEMBER, 6=SERVICE.
var standardRoleNames = map[int32]string{ //nolint:gochecknoglobals // enum mapping
	1: "owner",
	2: "admin",
	3: "operator",
	4: "viewer",
	5: "member",
	6: "service",
}

func standardRoleName(v int32) string {
	return standardRoleNames[v]
}

// publishManifestUntilSuccess blocks until the permission manifest is
// successfully registered. It retries indefinitely with exponential backoff
// because the service cannot enforce authorization without its namespace
// registered — starting without it would silently break all permission checks.
func publishManifestUntilSuccess(ctx context.Context, registrationURL string, manifest any) {
	logger := util.Log(ctx)

	body, err := json.Marshal(manifest)
	if err != nil {
		logger.WithError(err).Fatal("failed to marshal permission manifest")
		return
	}

	if m, ok := manifest.(map[string]any); ok {
		if ns, nsOK := m["namespace"].(string); nsOK {
			logger = logger.WithField("namespace", ns)
		}
	}

	backoff := initialBackoff
	for {
		if ctx.Err() != nil {
			logger.WithError(ctx.Err()).Fatal("context cancelled while waiting for permission manifest registration")
			return
		}

		if tryPublishManifest(ctx, registrationURL, body, logger) {
			return
		}

		logger.WithField("retry_in", backoff).Warn("permission manifest registration failed, retrying")
		time.Sleep(backoff)
		backoff = min(backoff*backoffMultiplier, maxBackoff)
	}
}

// tryPublishManifest attempts a single registration POST. Returns true on success.
func tryPublishManifest(ctx context.Context, registrationURL string, body []byte, logger *util.LogEntry) bool {
	const httpTimeout = 10 * time.Second
	client := &http.Client{Timeout: httpTimeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationURL, bytes.NewReader(body))
	if err != nil {
		logger.WithError(err).Warn("failed to create permission manifest request")
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).Warn("permission manifest request failed")
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode < http.StatusMultipleChoices {
		logger.Debug("permission manifest registered")
		return true
	}

	logger.WithField("status", resp.StatusCode).Warn("permission manifest registration returned non-success status")
	return false
}

// FormatNamespaceDisplay returns a human-readable name from a service
// namespace identifier (e.g. "service_profile" → "Profile").
func FormatNamespaceDisplay(namespace string) string {
	name := strings.TrimPrefix(namespace, "service_")
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return namespace
}
