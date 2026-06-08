package http

import (
	"net/http"

	"ov-dash/backend/internal/modules/notifications"
	"ov-dash/backend/internal/modules/proxy"
	"ov-dash/backend/internal/modules/registry"
	"ov-dash/backend/internal/modules/wiki"
	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
	platformsettings "ov-dash/backend/internal/platform/settings"
)

func defaultRegistry() *platformmodule.Registry {
	reg := registry.NewDefault()
	reg.Add(wikiHTTPModule{}, telegramNotificationsHTTPModule{})
	return reg
}

type moduleManifestResponse struct {
	ID           string                    `json:"id"`
	Capabilities []capability.Descriptor   `json:"capabilities"`
	Settings     []platformsettings.Schema `json:"settings,omitempty"`
}

type jobDefinitionResponse struct {
	Type           string `json:"type"`
	Description    string `json:"description"`
	TimeoutSeconds int64  `json:"timeout_seconds,omitempty"`
	MaxAttempts    int    `json:"max_attempts,omitempty"`
}

func platformModulesHandler(reg *platformmodule.Registry, jobs *platformmodule.JobRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modules := reg.Modules()
		items := make([]moduleManifestResponse, 0, len(modules))
		for _, module := range modules {
			item := moduleManifestResponse{ID: module.ID()}
			if capabilityModule, ok := module.(platformmodule.CapabilityModule); ok {
				item.Capabilities = capability.Descriptors(capabilityModule.Capabilities())
			}
			if settingsModule, ok := module.(platformmodule.SettingsModule); ok {
				item.Settings = settingsModule.SettingsSchemas()
			}
			items = append(items, item)
		}
		jobTypes := []jobDefinitionResponse{}
		if jobs != nil {
			for _, job := range jobs.Definitions() {
				jobTypes = append(jobTypes, jobDefinitionResponse{
					Type:           job.Type,
					Description:    job.Description,
					TimeoutSeconds: int64(job.Timeout.Seconds()),
					MaxAttempts:    job.MaxAttempts,
				})
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"items":        items,
			"capabilities": capability.Descriptors(reg.Capabilities()),
			"jobs":         jobTypes,
		})
	}
}

type wikiHTTPModule struct{}

func (wikiHTTPModule) ID() string {
	return "wiki"
}

func (wikiHTTPModule) RegisterHTTP(ctx platformmodule.Context) {
	handler := NewWikiHandler(
		wiki.NewService(wiki.NewRepository(ctx.DB)),
		ctx.Config.Uploads.WikiDir,
	)
	readWiki := ctx.RequireCapability(capability.WikiRead)
	writeWiki := ctx.RequireCapability(capability.WikiWrite)

	ctx.ProtectedRouter.With(readWiki).Get("/wiki/pages", handler.ListPages)
	ctx.ProtectedRouter.With(writeWiki).Post("/wiki/pages", handler.CreatePage)
	ctx.ProtectedRouter.With(readWiki).Get("/wiki/pages/{id}", handler.GetPage)
	ctx.ProtectedRouter.With(writeWiki).Put("/wiki/pages/{id}", handler.UpdatePage)
	ctx.ProtectedRouter.With(writeWiki).Delete("/wiki/pages/{id}", handler.DeletePage)
	ctx.ProtectedRouter.With(readWiki).Get("/wiki/pages/{id}/revisions", handler.ListRevisions)
	ctx.ProtectedRouter.With(writeWiki).Post("/wiki/attachments", handler.UploadAttachment)
	ctx.ProtectedRouter.With(readWiki).Get("/wiki/attachments/{id}/raw", handler.AttachmentRaw)
}

func (wikiHTTPModule) Capabilities() []capability.Capability {
	return []capability.Capability{
		capability.WikiRead,
		capability.WikiWrite,
	}
}

type telegramNotificationsHTTPModule struct{}

func (telegramNotificationsHTTPModule) ID() string {
	return "notifications"
}

func (telegramNotificationsHTTPModule) RegisterHTTP(ctx platformmodule.Context) {
	handler := NewTelegramNotificationsHandler(
		notifications.NewService(
			notifications.NewRepository(ctx.DB),
			notifications.NewProxiedTelegramClient(proxy.NewService(proxy.NewRepositoryWithSecrets(ctx.DB, ctx.Secrets))),
		),
	)
	readNotifications := ctx.RequireCapability(capability.NotificationsRead)
	writeNotifications := ctx.RequireCapability(capability.NotificationsWrite)

	ctx.PublicRouter.Post("/incoming-messages", handler.IncomingMessage)
	ctx.ProtectedRouter.With(readNotifications).Get("/telegram-notifications/settings", handler.GetSettings)
	ctx.ProtectedRouter.With(writeNotifications).Put("/telegram-notifications/settings", handler.UpdateSettings)
	ctx.ProtectedRouter.With(readNotifications).Get("/telegram-notifications/users", handler.ListUserSettings)
	ctx.ProtectedRouter.With(writeNotifications).Put("/telegram-notifications/users/{userID}", handler.UpdateUserSettings)
}

func (telegramNotificationsHTTPModule) Capabilities() []capability.Capability {
	return []capability.Capability{
		capability.NotificationsRead,
		capability.NotificationsWrite,
	}
}

func (telegramNotificationsHTTPModule) SettingsSchemas() []platformsettings.Schema {
	return []platformsettings.Schema{
		{
			Key:       "notifications.telegram.enabled",
			Type:      platformsettings.TypeBoolean,
			Default:   false,
			Writable:  true,
			Sensitive: false,
		},
		{
			Key:        "notifications.telegram.bot_token",
			Type:       platformsettings.TypeString,
			Default:    "",
			Writable:   true,
			Sensitive:  true,
			Validation: platformsettings.Validation{MaxLength: settingsIntPtr(4096)},
		},
		{
			Key:        "notifications.telegram.inbound_token",
			Type:       platformsettings.TypeString,
			Default:    "",
			Writable:   true,
			Sensitive:  true,
			Validation: platformsettings.Validation{MaxLength: settingsIntPtr(4096)},
		},
		{
			Key:       "notifications.telegram.group_enabled",
			Type:      platformsettings.TypeBoolean,
			Default:   false,
			Writable:  true,
			Sensitive: false,
		},
		{
			Key:        "notifications.telegram.group_chat_id",
			Type:       platformsettings.TypeString,
			Default:    "",
			Writable:   true,
			Sensitive:  false,
			Validation: platformsettings.Validation{MaxLength: settingsIntPtr(128)},
		},
	}
}

func settingsIntPtr(value int) *int {
	return &value
}
