package handler

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxAnnouncementImageUploadBytes = 8 << 20
	announcementImageURLPrefix      = "/api/v1/custom/announcements/images"
)

var announcementImageFilenamePattern = regexp.MustCompile(`^[0-9a-fA-F-]{36}\.(png|jpg|jpeg|gif|webp)$`)

// UploadHandler owns the custom announcement image upload surface.
//
// Announcement bodies are rendered for normal users, so uploaded images are stored as
// immutable files under the configured data directory and served through a random URL
// that does not require request headers.
type UploadHandler struct {
	imagesDir string
}

// ImageUploadResponse is returned after an admin uploads an announcement image.
type ImageUploadResponse struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

// NewUploadHandler creates an upload handler backed by data/custom/announcements/images.
func NewUploadHandler(dataDir string) *UploadHandler {
	baseDir := strings.TrimSpace(dataDir)
	if baseDir == "" {
		baseDir = "./data"
	}
	return &UploadHandler{
		imagesDir: filepath.Join(baseDir, "custom", "announcements", "images"),
	}
}

// UploadImage accepts one pasted or selected image and returns a Markdown-ready URL.
func (h *UploadHandler) UploadImage(c *gin.Context) {
	if h == nil {
		response.InternalError(c, "Announcement image upload is not available")
		return
	}

	// Bound this endpoint tighter than the global server cap so pasted images cannot
	// spill large multipart temp files before type validation runs.
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maxAnnouncementImageUploadBytes+(1<<20),
	)
	if err := c.Request.ParseMultipartForm(maxAnnouncementImageUploadBytes); err != nil {
		response.Error(c, http.StatusRequestEntityTooLarge, "Image upload is too large")
		return
	}

	file, _, err := c.Request.FormFile("image")
	if err != nil {
		response.BadRequest(c, "Image file is required")
		return
	}
	defer file.Close()

	body, err := io.ReadAll(io.LimitReader(file, maxAnnouncementImageUploadBytes+1))
	if err != nil {
		response.BadRequest(c, "Failed to read image file")
		return
	}
	if len(body) == 0 {
		response.BadRequest(c, "Image file is empty")
		return
	}
	if len(body) > maxAnnouncementImageUploadBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "Image upload is too large")
		return
	}

	contentType, ext, ok := detectAnnouncementImageType(body)
	if !ok {
		response.BadRequest(c, "Only PNG, JPEG, GIF and WebP images are supported")
		return
	}

	if err := os.MkdirAll(h.imagesDir, 0755); err != nil {
		response.InternalError(c, "Failed to prepare image storage")
		return
	}

	filename := uuid.NewString() + ext
	targetPath := filepath.Join(h.imagesDir, filename)
	if err := os.WriteFile(targetPath, body, 0644); err != nil {
		response.InternalError(c, "Failed to save image file")
		return
	}

	response.Success(c, ImageUploadResponse{
		URL:         announcementImageURLPrefix + "/" + filename,
		Filename:    filename,
		ContentType: contentType,
		Size:        len(body),
	})
}

// ServeImage returns an immutable uploaded announcement image by generated filename.
func (h *UploadHandler) ServeImage(c *gin.Context) {
	if h == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	filename := strings.TrimSpace(c.Param("filename"))
	if !announcementImageFilenamePattern.MatchString(filename) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	targetPath := filepath.Clean(filepath.Join(h.imagesDir, filename))
	if !isAnnouncementImagePathWithin(targetPath, h.imagesDir) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	info, err := os.Stat(targetPath)
	if err != nil || info.IsDir() {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	if contentType := announcementImageContentType(filename); contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.File(targetPath)
}

func detectAnnouncementImageType(body []byte) (contentType string, ext string, ok bool) {
	if len(body) >= 8 &&
		body[0] == 0x89 &&
		body[1] == 'P' &&
		body[2] == 'N' &&
		body[3] == 'G' &&
		body[4] == '\r' &&
		body[5] == '\n' &&
		body[6] == 0x1a &&
		body[7] == '\n' {
		return "image/png", ".png", true
	}
	if len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff {
		return "image/jpeg", ".jpg", true
	}
	if len(body) >= 6 && (string(body[:6]) == "GIF87a" || string(body[:6]) == "GIF89a") {
		return "image/gif", ".gif", true
	}
	if len(body) >= 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WEBP" {
		return "image/webp", ".webp", true
	}
	return "", "", false
}

func announcementImageContentType(filename string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

func isAnnouncementImagePathWithin(path string, base string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
