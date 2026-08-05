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

	"github.com/pitabwire/frame/v2/setup"
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

// WithPermissionRegistration registers a setup.Step named setup.NamePermissions
// that publishes this service's permission manifest(s) to tenancy.
//
// Runtime PreStart registration is intentionally not used — permission
// publishing is a one-shot setup plan step so process startup stays fast.
// Jobs must run `setup … permissions` (or legacy migrate via
// Service.RunSetupForProcess) with PERMISSIONS_REGISTRATION_URL set.
//
// Multiple descriptors may be passed in one call, or via repeated option
// applications — they accumulate and all publish under the single
// "permissions" setup step (no silent overwrite of earlier namespaces).
//
// See package setup and docs/SETUP_JOB.md.
//
// Usage:
//
//	sd := profilepb.File_profile_v1_profile_proto.Services().ByName("ProfileService")
//	svc.Setup().RegisterFunc(setup.NameMigrate, migrateFn)
//	svc.Init(ctx, frame.WithPermissionRegistration(sd))
//	// multi-service binary:
//	svc.Init(ctx, frame.WithPermissionRegistration(billingSD, collectionSD))
//	if frame.ShouldRunSetup(&cfg) {
//	    return svc.RunSetupForProcess(ctx, &cfg)
//	}
func WithPermissionRegistration(sds ...protoreflect.ServiceDescriptor) Option {
	return func(_ context.Context, s *Service) {
		registrationURL := os.Getenv(ManifestRegistrationURLEnvVar)
		if registrationURL == "" {
			return
		}
		appendPermissionDescriptors(s, sds)
		if len(s.permissionManifestSDs) == 0 {
			return
		}
		s.Setup().Register(setup.Func{
			StepName: setup.NamePermissions,
			Fn: func(ctx context.Context) error {
				return s.publishAllPermissionManifests(ctx, registrationURL)
			},
		})
	}
}

func appendPermissionDescriptors(s *Service, sds []protoreflect.ServiceDescriptor) {
	for _, sd := range sds {
		if sd != nil {
			s.permissionManifestSDs = append(s.permissionManifestSDs, sd)
		}
	}
}

// publishAllPermissionManifests posts every accumulated service descriptor's
// permission manifest to tenancy during the setup permissions step.
func (s *Service) publishAllPermissionManifests(ctx context.Context, registrationURL string) error {
	var firstErr error
	published := 0
	for _, sd := range s.permissionManifestSDs {
		if sd == nil {
			continue
		}
		manifest := buildManifestFromDescriptor(sd)
		if manifest == nil {
			util.Log(ctx).WithField("service", string(sd.FullName())).
				Warn("setup permissions: no service_permissions extension on descriptor; skipping")
			continue
		}
		if err := publishManifestWithRetrySync(ctx, s, registrationURL, manifest); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		published++
	}
	if published == 0 && firstErr == nil {
		util.Log(ctx).
			Warn("setup permissions: no manifests published (all descriptors lacked service_permissions)")
		return nil
	}
	if firstErr != nil {
		return firstErr
	}
	util.Log(ctx).WithField("manifests", published).Info("setup permissions: manifests registered")
	return nil
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

var permissionRegistrationDelays = []time.Duration{ //nolint:gochecknoglobals // shared backoff schedule
	1 * time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second,
	20 * time.Second, 30 * time.Second, 60 * time.Second,
}

// publishManifestWithRetrySync is the setup-job path: bounded backoff, fail-closed.
func publishManifestWithRetrySync(ctx context.Context, svc *Service, registrationURL string, manifest any) error {
	logger := util.Log(ctx)
	namespace := manifestNamespace(manifest)

	for attempt := range permissionRegistrationDelays {
		err := publishManifest(ctx, svc, registrationURL, manifest)
		if err == nil {
			if attempt > 0 {
				logger.WithField("namespace", namespace).WithField("attempts", attempt+1).
					Info("permission manifest registered after retries")
			}
			return nil
		}
		if done := waitPermissionRetry(ctx, logger, namespace, err, attempt); done != nil {
			return done
		}
	}
	return fmt.Errorf("permission manifest registration failed for %s after retries", namespace)
}

func manifestNamespace(manifest any) string {
	m, isMap := manifest.(map[string]any)
	if !isMap {
		return ""
	}
	ns, _ := m["namespace"].(string)
	return ns
}

// waitPermissionRetry sleeps for the next backoff slot. Returns a non-nil error
// when the caller should stop (context cancelled or retries exhausted).
func waitPermissionRetry(
	ctx context.Context,
	logger *util.LogEntry,
	namespace string,
	err error,
	attempt int,
) error {
	if ctx.Err() != nil {
		logger.WithError(err).WithField("namespace", namespace).
			Warn("permission manifest registration abandoned: context cancelled")
		return fmt.Errorf("permission manifest registration cancelled for %s: %w", namespace, err)
	}
	if attempt >= len(permissionRegistrationDelays)-1 {
		logger.WithError(err).WithField("namespace", namespace).
			Error("permission manifest registration failed after retries")
		return fmt.Errorf("permission manifest registration failed for %s: %w", namespace, err)
	}
	delay := permissionRegistrationDelays[attempt]
	logger.WithError(err).WithField("namespace", namespace).
		WithField("retry_in", delay.String()).
		Warn("permission manifest registration failed, retrying")

	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("permission manifest registration cancelled for %s: %w", namespace, err)
	case <-t.C:
		return nil
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
