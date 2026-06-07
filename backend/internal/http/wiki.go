package http

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ov-dash/backend/internal/modules/wiki"

	"github.com/go-chi/chi/v5"
)

const maxWikiAttachmentBytes = 10 * 1024 * 1024

type WikiHandler struct {
	service    *wiki.Service
	uploadsDir string
}

func NewWikiHandler(service *wiki.Service, uploadsDir string) *WikiHandler {
	uploadsDir = strings.TrimSpace(uploadsDir)
	if uploadsDir == "" {
		uploadsDir = "./uploads/wiki"
	}
	return &WikiHandler{service: service, uploadsDir: uploadsDir}
}

type saveWikiResourceRequest struct {
	ID           string `json:"id"`
	ResourceType string `json:"resource_type"`
	Title        string `json:"title"`
	Host         string `json:"host"`
	Port         string `json:"port"`
	URL          string `json:"url"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Note         string `json:"note"`
	SortOrder    int    `json:"sort_order"`
}

type saveWikiPageRequest struct {
	ParentID  string                    `json:"parent_id"`
	Title     string                    `json:"title"`
	PageType  string                    `json:"page_type"`
	Category  string                    `json:"category"`
	Summary   string                    `json:"summary"`
	ContentMD string                    `json:"content_md"`
	Tags      string                    `json:"tags"`
	Resources []saveWikiResourceRequest `json:"resources"`
}

type wikiAttachmentResponse struct {
	ID           string `json:"id"`
	PageID       string `json:"page_id"`
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	SizeBytes    int64  `json:"size_bytes"`
	URL          string `json:"url"`
	Markdown     string `json:"markdown"`
}

func (h *WikiHandler) ListPages(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "wiki_pages_list_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *WikiHandler) GetPage(w http.ResponseWriter, r *http.Request) {
	page, err := h.service.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeWikiError(w, err, "wiki_page_get_failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *WikiHandler) CreatePage(w http.ResponseWriter, r *http.Request) {
	input, ok := h.saveInputFromRequest(w, r)
	if !ok {
		return
	}
	page, err := h.service.Create(r.Context(), input)
	if err != nil {
		writeWikiError(w, err, "wiki_page_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, page)
}

func (h *WikiHandler) UpdatePage(w http.ResponseWriter, r *http.Request) {
	input, ok := h.saveInputFromRequest(w, r)
	if !ok {
		return
	}
	input.ID = strings.TrimSpace(chi.URLParam(r, "id"))
	page, err := h.service.Update(r.Context(), input)
	if err != nil {
		writeWikiError(w, err, "wiki_page_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *WikiHandler) DeletePage(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeWikiError(w, err, "wiki_page_delete_failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *WikiHandler) ListRevisions(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListRevisions(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeWikiError(w, err, "wiki_page_revisions_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *WikiHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWikiAttachmentBytes+1024*1024)
	if err := r.ParseMultipartForm(maxWikiAttachmentBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_multipart"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file_required"})
		return
	}
	defer file.Close()
	originalName := filepath.Base(header.Filename)

	data, err := io.ReadAll(io.LimitReader(file, maxWikiAttachmentBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file_read_failed"})
		return
	}
	if len(data) > maxWikiAttachmentBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file_too_large"})
		return
	}

	contentType, extension, ok := wikiImageType(data, header.Header.Get("Content-Type"))
	if !ok {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "unsupported_image_type"})
		return
	}
	if originalName == "" || originalName == "." {
		originalName = "image" + extension
	}

	attachmentID, err := wikiAttachmentID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "attachment_id_failed"})
		return
	}

	now := time.Now().UTC()
	storagePath := filepath.ToSlash(filepath.Join(
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		attachmentID+extension,
	))
	absolutePath := filepath.Join(h.uploadsDir, filepath.FromSlash(storagePath))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0750); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "attachment_dir_failed"})
		return
	}
	if err := os.WriteFile(absolutePath, data, 0640); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "attachment_write_failed"})
		return
	}

	attachment, err := h.service.CreateAttachment(r.Context(), wiki.SaveAttachmentInput{
		ID:           attachmentID,
		PageID:       r.FormValue("page_id"),
		OriginalName: originalName,
		StoragePath:  storagePath,
		ContentType:  contentType,
		SizeBytes:    int64(len(data)),
		ActorID:      user.ID,
	})
	if err != nil {
		_ = os.Remove(absolutePath)
		writeWikiError(w, err, "wiki_attachment_create_failed")
		return
	}

	writeJSON(w, http.StatusCreated, wikiAttachmentPayload(attachment))
}

func (h *WikiHandler) AttachmentRaw(w http.ResponseWriter, r *http.Request) {
	attachment, err := h.service.GetAttachment(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeWikiError(w, err, "wiki_attachment_get_failed")
		return
	}

	relativePath := filepath.FromSlash(attachment.StoragePath)
	if !filepath.IsLocal(relativePath) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid_attachment_path"})
		return
	}
	absolutePath := filepath.Join(h.uploadsDir, relativePath)
	file, err := os.Open(absolutePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "wiki_attachment_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "wiki_attachment_open_failed"})
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "wiki_attachment_stat_failed"})
		return
	}

	filename := attachment.OriginalName
	if filename == "" {
		filename = attachment.ID
	}
	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	http.ServeContent(w, r, filename, stat.ModTime(), file)
}

func (h *WikiHandler) saveInputFromRequest(w http.ResponseWriter, r *http.Request) (wiki.SavePageInput, bool) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return wiki.SavePageInput{}, false
	}

	var payload saveWikiPageRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return wiki.SavePageInput{}, false
	}

	resources := make([]wiki.SaveResourceInput, 0, len(payload.Resources))
	for _, resource := range payload.Resources {
		resources = append(resources, wiki.SaveResourceInput{
			ID:           resource.ID,
			ResourceType: resource.ResourceType,
			Title:        resource.Title,
			Host:         resource.Host,
			Port:         resource.Port,
			URL:          resource.URL,
			Username:     resource.Username,
			Password:     resource.Password,
			Note:         resource.Note,
			SortOrder:    resource.SortOrder,
		})
	}

	return wiki.SavePageInput{
		ParentID:  payload.ParentID,
		Title:     payload.Title,
		PageType:  payload.PageType,
		Category:  payload.Category,
		Summary:   payload.Summary,
		ContentMD: payload.ContentMD,
		Tags:      payload.Tags,
		Resources: resources,
		ActorID:   user.ID,
	}, true
}

func writeWikiError(w http.ResponseWriter, err error, fallback string) {
	status := http.StatusInternalServerError
	code := fallback

	switch {
	case errors.Is(err, wiki.ErrTitleRequired),
		errors.Is(err, wiki.ErrIDRequired),
		errors.Is(err, wiki.ErrFileRequired):
		status = http.StatusBadRequest
		code = err.Error()
	case wiki.IsNotFound(err):
		status = http.StatusNotFound
		code = "wiki_page_not_found"
	}

	writeJSON(w, status, map[string]string{"error": code})
}

func wikiAttachmentPayload(attachment wiki.Attachment) wikiAttachmentResponse {
	url := fmt.Sprintf("/api/v1/wiki/attachments/%s/raw", attachment.ID)
	return wikiAttachmentResponse{
		ID:           attachment.ID,
		PageID:       attachment.PageID,
		OriginalName: attachment.OriginalName,
		ContentType:  attachment.ContentType,
		SizeBytes:    attachment.SizeBytes,
		URL:          url,
		Markdown:     fmt.Sprintf("![%s](%s)", markdownAlt(attachment.OriginalName), url),
	}
}

func wikiImageType(data []byte, fallback string) (string, string, bool) {
	allowed := map[string]string{
		"image/gif":  ".gif",
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	}

	if len(data) > 0 {
		contentType := http.DetectContentType(data)
		if extension, ok := allowed[contentType]; ok {
			return contentType, extension, true
		}
	}

	contentType := strings.ToLower(strings.TrimSpace(strings.Split(fallback, ";")[0]))
	if extension, ok := allowed[contentType]; ok {
		return contentType, extension, true
	}
	return "", "", false
}

func markdownAlt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "图片"
	}
	value = strings.ReplaceAll(value, "]", "")
	value = strings.ReplaceAll(value, "[", "")
	return strings.ReplaceAll(value, "\n", " ")
}

func wikiAttachmentID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
