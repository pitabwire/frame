package frame

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pitabwire/util"
	"google.golang.org/protobuf/reflect/protoreflect"
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

// ManifestRegistrationPath is the default HTTP path for the internal
// permission manifest registration endpoint on the tenancy service.
const ManifestRegistrationPath = "/_internal/register/permissions"

// WithPermissionRegistration registers a service's permission manifest with
// the tenancy service. The manifest (namespace, permissions, role bindings)
// is extracted from the proto service descriptor's service_permissions
// annotation using proto reflection.
//
// Registration runs as a PreStartMethod whenever PERMISSIONS_REGISTRATION_URL
// is set and the proto descriptor carries a service_permissions extension.
// It fires once per process start, before the service begins serving
// traffic. The tenancy registration endpoint is an idempotent upsert keyed
// on namespace, so repeat registrations across pod restarts are safe and
// keep the tenancy.service_namespaces table in sync with whatever the
// running binary declares.
//
// Earlier versions gated registration on DO_MIGRATION=true so that only the
// migration job published manifests. That coupling broke whenever a
// service's cmd/main.go short-circuited before svc.Run (typical pattern):
// the migration pod never reached the PreStartMethod, the regular pod was
// locked out by the gate, and the namespace was never registered. Decoupling
// from migration mode fixes that without forcing every consumer to invent
// glue around svc.Run.
//
// The tenancy service URL is read from PERMISSIONS_REGISTRATION_URL. When
// unset, the option is a no-op — services that don't want to register can
// leave it unset in their environment.
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

		s.AddPreStartMethod(func(ctx context.Context, svc *Service) {
			manifest := buildManifestFromDescriptor(sd)
			if manifest == nil {
				return
			}

			// Retry the manifest POST in a small backoff loop. A pod that is
			// also its own OAuth2 token-signing endpoint (the auth service)
			// can't sign assertions until at least one replica is ready, so
			// the first few attempts during a cold-start roll may legitimately
			// see connection-refused / token-endpoint-unavailable. Logging at
			// Warn (rather than Fatal-ing the process) keeps the pod alive
			// long enough for the signer to come up; once registration
			// succeeds the loop exits. Registration is an idempotent upsert,
			// so retrying is safe.
			go publishManifestWithRetry(ctx, svc, registrationURL, manifest)
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

// publishManifestWithRetry runs publishManifest in a bounded backoff loop.
// It logs failures at Warn level instead of crashing the process so a pod
// that depends on its own OAuth2 token-signing endpoint (the auth service)
// can survive the brief window during a cold rollout when no replica is
// available to sign assertions. The endpoint is an idempotent upsert keyed
// on namespace, so retried POSTs after success are harmless.
func publishManifestWithRetry(ctx context.Context, svc *Service, registrationURL string, manifest any) {
	logger := util.Log(ctx)
	namespace := ""
	if m, ok := manifest.(map[string]any); ok {
		namespace, _ = m["namespace"].(string)
	}

	delays := []time.Duration{
		1 * time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second,
		20 * time.Second, 30 * time.Second, 60 * time.Second,
	}
	for attempt := 0; ; attempt++ {
		err := publishManifest(ctx, svc, registrationURL, manifest)
		if err == nil {
			if attempt > 0 {
				logger.WithField("namespace", namespace).WithField("attempts", attempt+1).
					Info("permission manifest registered after retries")
			}
			return
		}
		if ctx.Err() != nil {
			logger.WithError(err).WithField("namespace", namespace).
				Warn("permission manifest registration abandoned: context cancelled")
			return
		}
		delay := delays[len(delays)-1]
		if attempt < len(delays) {
			delay = delays[attempt]
		}
		logger.WithError(err).WithField("namespace", namespace).
			WithField("retry_in", delay.String()).
			Warn("permission manifest registration failed, retrying")

		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// publishManifest registers the permission manifest using the service's
// internal HTTP client. Returns an error if registration fails — used by
// publishManifestWithRetry.
func publishManifest(ctx context.Context, svc *Service, registrationURL string, manifest any) error {
	logger := util.Log(ctx)

	namespace := ""
	if m, ok := manifest.(map[string]any); ok {
		namespace, _ = m["namespace"].(string)
	}

	// Pass the manifest directly — Invoke marshals JSON internally. Passing a
	// pre-marshalled []byte would be re-marshalled, producing a base64-encoded
	// string that the tenancy registration endpoint cannot decode.
	resp, err := svc.HTTPClientManager().Invoke(ctx, http.MethodPost, registrationURL, manifest, nil)
	if err != nil {
		return fmt.Errorf("permission manifest registration request failed for %s: %w", namespace, err)
	}

	if resp.Body != nil {
		defer util.CloseAndLogOnError(ctx, resp.Body)
	}

	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("permission manifest registration for %s returned status %d", namespace, resp.StatusCode)
	}

	logger.WithField("namespace", namespace).Debug("permission manifest registered")
	return nil
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
