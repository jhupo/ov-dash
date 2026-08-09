package notifications

import (
	"ov-dash/backend/internal/modules/proxy"
	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
	"ov-dash/backend/internal/platform/settings"
)

type Module struct{}

func NewModule() Module {
	return Module{}
}

func (Module) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "notifications",
		Title:       "Notifications",
		Description: "Telegram notification settings and inbound webhook API.",
		Kind:        "service",
		Tags:        []string{"notifications", "telegram", "settings"},
		DependsOn:   []string{"proxy"},
	}
}

func (Module) Register(reg *platformmodule.Registrar) error {
	if err := reg.Capabilities(capability.NotificationsRead, capability.NotificationsWrite); err != nil {
		return err
	}
	if err := reg.Settings(settingsSchemas()...); err != nil {
		return err
	}
	return reg.HTTP(func(ctx platformmodule.Context) {
		handler := NewTelegramNotificationsHandler(NewService(
			NewRepository(ctx.DB, ctx.Secrets),
			NewProxiedTelegramClient(proxy.NewService(proxy.NewRepository(ctx.DB, ctx.Secrets))),
		))
		readNotifications := ctx.RequireCapability(capability.NotificationsRead)
		writeNotifications := ctx.RequireCapability(capability.NotificationsWrite)

		ctx.PublicRouter.Post("/incoming-messages", handler.IncomingMessage)
		ctx.ProtectedRouter.With(readNotifications).Get("/telegram-notifications/settings", handler.GetSettings)
		ctx.ProtectedRouter.With(writeNotifications).Put("/telegram-notifications/settings", handler.UpdateSettings)
		ctx.ProtectedRouter.With(readNotifications).Get("/telegram-notifications/users", handler.ListUserSettings)
		ctx.ProtectedRouter.With(writeNotifications).Put("/telegram-notifications/users/{userID}", handler.UpdateUserSettings)
	})
}

func settingsSchemas() []settings.Schema {
	return []settings.Schema{
		{
			Key:       "notifications.telegram.enabled",
			Type:      settings.TypeBoolean,
			Default:   false,
			Writable:  true,
			Sensitive: false,
		},
		{
			Key:        "notifications.telegram.bot_token",
			Type:       settings.TypeString,
			Default:    "",
			Writable:   true,
			Sensitive:  true,
			Validation: settings.Validation{MaxLength: intPtr(4096)},
		},
		{
			Key:        "notifications.telegram.inbound_token",
			Type:       settings.TypeString,
			Default:    "",
			Writable:   true,
			Sensitive:  true,
			Validation: settings.Validation{MaxLength: intPtr(4096)},
		},
		{
			Key:       "notifications.telegram.group_enabled",
			Type:      settings.TypeBoolean,
			Default:   false,
			Writable:  true,
			Sensitive: false,
		},
		{
			Key:        "notifications.telegram.group_chat_id",
			Type:       settings.TypeString,
			Default:    "",
			Writable:   true,
			Sensitive:  false,
			Validation: settings.Validation{MaxLength: intPtr(128)},
		},
	}
}

func intPtr(value int) *int {
	return &value
}
