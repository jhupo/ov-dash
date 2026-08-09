package wiki

import (
	"ov-dash/backend/internal/platform/capability"
	platformmodule "ov-dash/backend/internal/platform/module"
)

type Module struct{}

func NewModule() Module {
	return Module{}
}

func (Module) Manifest() platformmodule.Manifest {
	return platformmodule.Manifest{
		ID:          "wiki",
		Title:       "Wiki",
		Description: "Wiki pages, revisions, and attachment API.",
		Kind:        "service",
		Tags:        []string{"wiki", "content"},
	}
}

func (Module) Register(reg *platformmodule.Registrar) error {
	if err := reg.Capabilities(capability.WikiRead, capability.WikiWrite); err != nil {
		return err
	}
	return reg.HTTP(func(ctx platformmodule.Context) {
		handler := NewWikiHandler(NewService(NewRepository(ctx.DB, ctx.Secrets)), ctx.Config.Uploads.WikiDir)
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
	})
}
