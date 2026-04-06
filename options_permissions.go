package frame

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// maxRetries is the number of publish attempts for permission manifest registration.
const maxRetries = 3

// ManifestRegistrationPath is the default HTTP path for the internal
// permission manifest registration endpoint on the tenancy service.
const ManifestRegistrationPath = "/_internal/register/permissions"

// WithPermissionRegistration registers a service's permission manifest with
// the tenancy service at startup. The manifest (namespace, permissions,
// role bindings) is extracted from the proto service descriptor's
// service_permissions annotation using proto reflection — no typed imports
// needed.
//
// Registration is asynchronous and best-effort — it does not block service
// startup or cause failures if the tenancy service is unavailable. The
// tenancy service URL is read from PERMISSIONS_REGISTRATION_URL.
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

		s.AddPreStartMethod(func(ctx context.Context, _ *Service) {
			manifest := buildManifestFromDescriptor(sd)
			if manifest == nil {
				return
			}
			go publishManifest(ctx, registrationURL, manifest)
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

func publishManifest(ctx context.Context, registrationURL string, manifest any) {
	logger := util.Log(ctx)

	body, err := json.Marshal(manifest)
	if err != nil {
		logger.WithError(err).Warn("failed to marshal permission manifest")
		return
	}

	if m, ok := manifest.(map[string]any); ok {
		if ns, nsOK := m["namespace"].(string); nsOK {
			logger = logger.WithField("namespace", ns)
		}
	}

	const httpTimeout = 10 * time.Second
	client := &http.Client{Timeout: httpTimeout}

	for attempt := range maxRetries {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, registrationURL, bytes.NewReader(body))
		if reqErr != nil {
			logger.WithError(reqErr).Warn("failed to create permission manifest request")
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, doErr := client.Do(req)
		if doErr != nil {
			if attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
				continue
			}
			logger.WithError(doErr).Warn("failed to publish permission manifest")
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= http.StatusMultipleChoices {
			logger.WithField("status", resp.StatusCode).
				Warn(fmt.Sprintf("permission manifest registration returned %d", resp.StatusCode))
			return
		}

		logger.Debug("permission manifest registered")
		return
	}
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
