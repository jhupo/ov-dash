package http

import (
	"net/http"

	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
	platformsettings "ov-dash/backend/internal/platform/settings"
)

type moduleManifestResponse struct {
	ID           string                    `json:"id"`
	Title        string                    `json:"title"`
	Description  string                    `json:"description,omitempty"`
	Kind         string                    `json:"kind"`
	Tags         []string                  `json:"tags,omitempty"`
	Capabilities []capability.Descriptor   `json:"capabilities"`
	Jobs         []jobDefinitionResponse   `json:"jobs,omitempty"`
	HealthChecks []healthCheckResponse     `json:"health_checks,omitempty"`
	Settings     []platformsettings.Schema `json:"settings,omitempty"`
}

type jobDefinitionResponse struct {
	Type           string `json:"type"`
	Description    string `json:"description,omitempty"`
	TimeoutSeconds int64  `json:"timeout_seconds,omitempty"`
	MaxAttempts    int    `json:"max_attempts,omitempty"`
	ModuleID       string `json:"module_id,omitempty"`
}

type healthCheckResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func platformModulesHandler(catalog *platformmodule.Catalog) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		descriptors := catalog.Descriptors()
		items := make([]moduleManifestResponse, 0, len(descriptors))
		for _, descriptor := range descriptors {
			items = append(items, moduleManifestFromDescriptor(descriptor))
		}
		jobTypes := []jobDefinitionResponse{}
		if jobs := catalog.Jobs(); jobs != nil {
			for _, job := range jobs.Definitions() {
				jobTypes = append(jobTypes, jobDefinitionFromDescriptor(platformmodule.DescribeJob(job)))
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"items":        items,
			"capabilities": capability.Descriptors(catalog.Capabilities()),
			"jobs":         jobTypes,
		})
	}
}

func moduleManifestFromDescriptor(descriptor platformmodule.ModuleDescriptor) moduleManifestResponse {
	item := moduleManifestResponse{
		ID:           descriptor.ID,
		Title:        descriptor.Title,
		Description:  descriptor.Description,
		Kind:         descriptor.Kind,
		Tags:         descriptor.Tags,
		Capabilities: descriptor.Capabilities,
		Settings:     descriptor.Settings,
	}
	for _, job := range descriptor.Jobs {
		item.Jobs = append(item.Jobs, jobDefinitionFromDescriptor(job))
	}
	for _, check := range descriptor.HealthChecks {
		item.HealthChecks = append(item.HealthChecks, healthCheckResponse{ID: check.ID, Name: check.Name})
	}
	return item
}

func jobDefinitionFromDescriptor(descriptor platformmodule.JobDescriptor) jobDefinitionResponse {
	return jobDefinitionResponse{
		Type:           descriptor.Type,
		Description:    descriptor.Description,
		TimeoutSeconds: descriptor.TimeoutSeconds,
		MaxAttempts:    descriptor.MaxAttempts,
		ModuleID:       descriptor.ModuleID,
	}
}
