package capability

import (
	"context"
	"fmt"
	"slices"

	"measix/platform/ent/workspaceservice"
	"measix/platform/internal/wire/adminapi"
)

func (s *Service) validateDerivedDAVSave(ctx context.Context, content adminapi.ManagedDraftContent) error {
	services, err := s.Client.WorkspaceService.Query().Where(workspaceservice.DavStagedEQ(true)).All(ctx)
	if err != nil {
		return err
	}
	for _, service := range services {
		definitions := webdavDefinitions(content)
		if len(definitions) != 1 || definitions[0].ResourceId != service.DavResourceID {
			return fmt.Errorf("%w: the workspace DAV definition must be retained; disable it instead", ErrInvalidDraft)
		}
		found := false
		for _, binding := range content.Bindings {
			if binding.ResourceId != service.DavResourceID {
				continue
			}
			if found || s.validateDAVBinding(ctx, binding) != nil {
				return fmt.Errorf("%w: derived DAV binding is immutable", ErrInvalidDraft)
			}
			found = true
		}
		if !found {
			return fmt.Errorf("%w: derived DAV binding must be retained", ErrInvalidDraft)
		}
	}
	return nil
}

// DAVMethods lists the supported RFC 4918 HTTP methods, including HTTP reads.
// This is a route envelope; each request is further restricted by user access.
func DAVMethods() []string {
	return []string{"OPTIONS", "GET", "HEAD", "PROPFIND", "PUT", "DELETE", "MKCOL", "COPY", "MOVE", "PROPPATCH", "LOCK", "UNLOCK"}
}

func (s *Service) validateDAVBinding(ctx context.Context, binding adminapi.RuntimeBindingDefinition) error {
	if binding.TargetKind == nil || *binding.TargetKind != "REMOTE_WORKSPACE" || binding.WorkspaceServiceId == nil || binding.UpstreamId != "" {
		return fmt.Errorf("DAV must reference its remote workspace without an upstream")
	}
	row, err := s.Client.WorkspaceService.Get(ctx, *binding.WorkspaceServiceId)
	if err != nil || row.DavResourceID != binding.ResourceId || row.DavRuntimeRouteID != binding.RuntimeRouteId || row.ActiveConfigRevision == nil {
		return fmt.Errorf("DAV requires its configured workspace and stable route")
	}
	if binding.TransportPolicy != "HTTP_REQUEST_RESPONSE" || len(binding.AllowedPathPrefixes) != 1 || binding.AllowedPathPrefixes[0] != "/dav" {
		return fmt.Errorf("DAV uses the fixed streaming HTTP endpoint")
	}
	methods := DAVMethods()
	if len(binding.AllowedMethods) != len(methods) {
		return fmt.Errorf("DAV binding must retain all supported standard methods")
	}
	seen := make(map[string]bool)
	for _, method := range binding.AllowedMethods {
		if seen[method] || !slices.Contains(methods, method) {
			return fmt.Errorf("DAV binding has unsupported or duplicate method")
		}
		seen[method] = true
	}
	return nil
}

// DAVRootPathTemplate is bound to the caller's space by the dynamic workspace
// projection. It never contains an external address or upstream credential.
func DAVRootPathTemplate(resourceID string) string {
	return "/runtime/v1/resources/" + resourceID + "/spaces/{agentSpaceId}/dav/"
}

func webdavDefinitions(content adminapi.ManagedDraftContent) []adminapi.WebDAVDefinition {
	if content.Webdav == nil {
		return nil
	}
	return *content.Webdav
}
