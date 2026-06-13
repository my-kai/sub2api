package chatgpt2api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestGenerateImageCallsOpenAICompatibleEndpoint 固化 chatgpt2api 文生图开放接口契约。
func TestGenerateImageCallsOpenAICompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-key" {
			t.Fatalf("Authorization = %q", got)
		}
		var payload ImageGenerationRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "gpt-image-2" || payload.N != 2 || payload.ResponseFormat != "url" {
			t.Fatalf("payload = %+v", payload)
		}
		if payload.Quality != "high" || payload.Size != "1024x1536" {
			t.Fatalf("payload = %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://2api.example/images/1.png"}]}`))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	result, err := client.GenerateImage(t.Context(), ImageGenerationRequest{Prompt: "cat", N: 2, Quality: "high", Size: "1024x1536"})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].URL == "" {
		t.Fatalf("result = %+v", result)
	}
}

// TestGenerateImageAllowsExtendedResolutionSizes 覆盖 chatgpt2api 当前开放的 2K / 4K 尺寸透传。
func TestGenerateImageAllowsExtendedResolutionSizes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload ImageGenerationRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Size != "3840x2160" {
			t.Fatalf("payload = %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://2api.example/images/2.png"}]}`))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	if _, err := client.GenerateImage(t.Context(), ImageGenerationRequest{Prompt: "cat", Size: "3840x2160"}); err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
}

// TestEditImageDownloadsSourceImageBeforeMultipartEndpoint 固化编辑链路在本服务内下载来源图，避开 2api 固定 60 秒 image_url 拉图超时。
func TestEditImageDownloadsSourceImageBeforeMultipartEndpoint(t *testing.T) {
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "image/*,*/*;q=0.8" {
			t.Fatalf("source Accept = %q", got)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("source-image"))
	}))
	defer sourceServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-key" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data") {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if r.Form.Get("model") != "gpt-image-2" || r.Form.Get("prompt") != "make it blue" || r.Form.Get("n") != "2" {
			t.Fatalf("form = %+v", r.Form)
		}
		if r.Form.Get("quality") != "low" || r.Form.Get("size") != "1024x1024" || r.Form.Get("response_format") != "url" {
			t.Fatalf("form = %+v", r.Form)
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("FormFile(image) error = %v", err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll(image) error = %v", err)
		}
		if string(body) != "source-image" {
			t.Fatalf("image body = %q", string(body))
		}
		if got := header.Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("image Content-Type = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://2api.example/images/edit.png"}]}`))
	}))
	defer apiServer.Close()

	baseURL, err := url.Parse(apiServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	result, err := client.EditImage(t.Context(), ImageEditRequest{
		Prompt:        "make it blue",
		N:             2,
		Quality:       "low",
		Size:          "1024x1024",
		ImageURL:      sourceServer.URL + "/images/source.png",
		ImageFilename: "source.png",
	})
	if err != nil {
		t.Fatalf("EditImage() error = %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].URL == "" {
		t.Fatalf("result = %+v", result)
	}
}

// TestEditImageKeepsMultipartFileFallback 保留 multipart 文件上传兜底，避免外部调用方被破坏。
func TestEditImageKeepsMultipartFileFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("FormFile(image) error = %v", err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll(image) error = %v", err)
		}
		if string(body) != "source-image" {
			t.Fatalf("image body = %q", string(body))
		}
		if got := header.Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("image Content-Type = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://2api.example/images/edit-file.png"}]}`))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	result, err := client.EditImage(t.Context(), ImageEditRequest{
		Prompt:        "make it blue",
		ImageBytes:    []byte("source-image"),
		ImageFilename: "source.png",
	})
	if err != nil {
		t.Fatalf("EditImage() error = %v", err)
	}
	if len(result.Data) != 1 || result.Data[0].URL == "" {
		t.Fatalf("result = %+v", result)
	}
}

// TestEditImageValidatesSourceImage 避免空图片编辑请求打到上游。
func TestEditImageValidatesSourceImage(t *testing.T) {
	baseURL, err := url.Parse("http://127.0.0.1:8000")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	if _, err := client.EditImage(t.Context(), ImageEditRequest{Prompt: "cat"}); err == nil {
		t.Fatal("expected empty image to fail")
	}
}

// TestGenerateImageValidatesPromptAndCount 避免无效请求打到上游消耗额度。
func TestGenerateImageValidatesPromptAndCount(t *testing.T) {
	baseURL, err := url.Parse("http://127.0.0.1:8000")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	if _, err := client.GenerateImage(t.Context(), ImageGenerationRequest{Prompt: "   "}); err == nil {
		t.Fatal("expected empty prompt to fail")
	}
	if _, err := client.GenerateImage(t.Context(), ImageGenerationRequest{Prompt: "cat", N: 11}); err == nil {
		t.Fatal("expected oversized n to fail")
	}
}

// TestGenerateImageReturnsEmptyArrayForNullData 保护前端 JSON 数组合约。
func TestGenerateImageReturnsEmptyArrayForNullData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	result, err := client.GenerateImage(t.Context(), ImageGenerationRequest{Prompt: "cat"})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if result.Data == nil || len(result.Data) != 0 {
		t.Fatalf("Data = %#v", result.Data)
	}
}

// TestGenerateImageMapsUnauthorized 确认 auth-key 异常不会被误报为普通解析失败。
func TestGenerateImageMapsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "bad-key", time.Second)

	if _, err := client.GenerateImage(t.Context(), ImageGenerationRequest{Prompt: "cat"}); err != ErrUnauthorized {
		t.Fatalf("err = %v", err)
	}
}

// TestModelsNormalizesNullData 确认模型列表空值也返回 JSON 数组。
func TestModelsNormalizesNullData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/models") {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	models, err := client.Models(t.Context())
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if models.Data == nil {
		t.Fatal("models.Data is nil")
	}
}

// TestEndpointURLAvoidsDuplicateV1 兼容文档常见的 CHATGPT2API_BASE_URL=http://host/v1 写法。
func TestEndpointURLAvoidsDuplicateV1(t *testing.T) {
	baseURL, err := url.Parse("http://127.0.0.1:8000/v1")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(baseURL, "image-key", time.Second)

	if got := client.endpointURL("/v1/images/generations"); got != "http://127.0.0.1:8000/v1/images/generations" {
		t.Fatalf("endpointURL() = %q", got)
	}
}

// TestGenerateImageUsesRuntimeConfigLoader 确认 worker 每次调用都读取 DB 中的最新上游配置。
func TestGenerateImageUsesRuntimeConfigLoader(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://2api.example/images/runtime.png"}]}`))
	}))
	defer server.Close()

	runtimeBaseURL, err := url.Parse(server.URL + "/v1")
	if err != nil {
		t.Fatal(err)
	}
	staticBaseURL, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(staticBaseURL, "static-key", time.Second).WithConfigLoader(func(ctx context.Context) (RuntimeConfig, error) {
		return RuntimeConfig{BaseURL: runtimeBaseURL, AuthKey: "runtime-key"}, nil
	})

	if !client.Configured() {
		t.Fatal("Configured() should use runtime loader")
	}
	if _, err := client.GenerateImage(t.Context(), ImageGenerationRequest{Prompt: "cat"}); err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if gotAuth != "Bearer runtime-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}
