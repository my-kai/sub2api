package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUploadImageStoresSupportedImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler(t.TempDir())
	router := gin.New()
	router.POST("/upload", handler.UploadImage)

	body, contentType := buildMultipartImage(t, "image", "pasted.png", []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'd', 'a', 't', 'a',
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var payload struct {
		Code int `json:"code"`
		Data struct {
			URL         string `json:"url"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Size        int    `json:"size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, 0, payload.Code)
	require.Equal(t, "image/png", payload.Data.ContentType)
	require.True(t, strings.HasPrefix(payload.Data.URL, announcementImageURLPrefix+"/"))
	require.FileExists(t, filepath.Join(handler.imagesDir, payload.Data.Filename))
}

func TestUploadImageRejectsUnsupportedImage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler(t.TempDir())
	router := gin.New()
	router.POST("/upload", handler.UploadImage)

	body, contentType := buildMultipartImage(t, "image", "note.txt", []byte("not an image"))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestServeImageRejectsTraversalFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewUploadHandler(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/image", nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = req
	ctx.Params = gin.Params{{Key: "filename", Value: "../secret.png"}}

	handler.ServeImage(ctx)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func buildMultipartImage(t *testing.T, fieldName string, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return &body, writer.FormDataContentType()
}
