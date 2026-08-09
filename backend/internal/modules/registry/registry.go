package registry

import (
	"ov-dash/backend/internal/modules/apps"
	"ov-dash/backend/internal/modules/auth"
	"ov-dash/backend/internal/modules/chats"
	"ov-dash/backend/internal/modules/dashboard"
	"ov-dash/backend/internal/modules/jobs"
	"ov-dash/backend/internal/modules/notifications"
	"ov-dash/backend/internal/modules/platforminfo"
	"ov-dash/backend/internal/modules/proxy"
	"ov-dash/backend/internal/modules/servers"
	"ov-dash/backend/internal/modules/tasks"
	"ov-dash/backend/internal/modules/updates"
	"ov-dash/backend/internal/modules/users"
	"ov-dash/backend/internal/modules/wiki"
	platformmodule "ov-dash/backend/internal/platform/module"
)

func Default() []platformmodule.Module {
	return []platformmodule.Module{
		auth.NewModule(),
		updates.NewModule(),
		jobs.NewModule(),
		platforminfo.NewModule(),
		dashboard.NewModule(),
		tasks.NewModule(),
		users.NewModule(),
		apps.NewModule(),
		chats.NewModule(),
		proxy.NewModule(),
		notifications.NewModule(),
		servers.NewModule(),
		wiki.NewModule(),
	}
}
