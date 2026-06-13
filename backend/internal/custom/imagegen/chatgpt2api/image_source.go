package chatgpt2api

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
)

// downloadEditSourceImage 把编辑来源图在本服务内下载成 multipart 文件。
//
// basketikun/chatgpt2api 当前对 JSON image_url 拉图使用固定 60 秒超时；这里复用本服务的
// IMAGE_CLIENT_TIMEOUT，避免慢速图片下载在进入真正生图前被上游固定超时截断。
func (c *Client) downloadEditSourceImage(ctx context.Context, input ImageEditRequest) (ImageEditRequest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.ImageURL, nil)
	if err != nil {
		return ImageEditRequest{}, fmt.Errorf("%w: build source image request: %v", ErrInvalidRequest, err)
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	req.Header.Set("User-Agent", "sub2api-ex image source fetcher")

	resp, err := c.sourceDownloadClient.Do(req)
	if err != nil {
		return ImageEditRequest{}, fmt.Errorf("%w: source image download failed: %v", ErrInvalidRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ImageEditRequest{}, fmt.Errorf("%w: source image download failed: HTTP %d", ErrInvalidRequest, resp.StatusCode)
	}

	body, err := readLimitedImageBody(resp.Body)
	if err != nil {
		return ImageEditRequest{}, err
	}
	contentType := detectImageContentType(resp.Header.Get("Content-Type"), body)
	if !strings.HasPrefix(contentType, "image/") {
		return ImageEditRequest{}, fmt.Errorf("%w: source image must be an image", ErrInvalidRequest)
	}
	filename := input.ImageFilename
	if filename == defaultImageFilename {
		filename = filenameFromSourceURL(input.ImageURL, contentType)
	}
	input.ImageBytes = body
	input.ImageFilename = filename
	input.ImageContentType = contentType
	input.ImageURL = ""
	return input, nil
}

// readLimitedImageBody 读取来源图片并限制大小，避免慢速或异常大响应撑爆 worker 内存。
func readLimitedImageBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxImageBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read source image: %v", ErrInvalidRequest, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: source image is empty", ErrInvalidRequest)
	}
	if len(data) > maxImageBodySize {
		return nil, fmt.Errorf("%w: image is too large", ErrInvalidRequest)
	}
	return data, nil
}

// detectImageContentType 优先尊重图片响应头，缺失时用文件签名推断，兼容 2api 图片直链。
func detectImageContentType(header string, body []byte) string {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(header, ";")[0]))
	if strings.HasPrefix(contentType, "image/") {
		return contentType
	}
	return http.DetectContentType(body)
}

// filenameFromSourceURL 从来源链接推导文件名，兜底补扩展名便于上游识别 multipart 图片。
func filenameFromSourceURL(raw string, contentType string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return defaultImageFilename
	}
	filename := strings.TrimSpace(path.Base(parsed.EscapedPath()))
	if filename == "" || filename == "." || filename == "/" {
		filename = "source"
	}
	filename, _ = url.PathUnescape(filename)
	if !strings.Contains(filename, ".") {
		if extensions, err := mime.ExtensionsByType(contentType); err == nil && len(extensions) > 0 {
			filename += extensions[0]
		} else {
			filename += ".png"
		}
	}
	return filename
}

// createImageFormFile 显式构造 multipart 文件头，避免 CreateFormFile 默认 octet-stream 影响上游解析。
func createImageFormFile(writer *multipart.Writer, fieldName string, filename string, contentType string) (io.Writer, error) {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     fieldName,
		"filename": filename,
	}))
	header.Set("Content-Type", normalizeImageContentType(contentType))
	return writer.CreatePart(header)
}
