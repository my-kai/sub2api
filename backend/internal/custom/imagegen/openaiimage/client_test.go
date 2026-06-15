package openaiimage

import (
	"encoding/base64"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientGenerateImagePostsOfficialPathAndNormalizesB64JSON(t *testing.T) {
	var gotAuth string
	var gotPath string
	var payload GenerateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("png")) + `","revised_prompt":"better"}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL+"/v1", "secret-key", time.Second, nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	result, err := client.GenerateImage(t.Context(), GenerateRequest{
		Model:   "gpt-image-2",
		Prompt:  "cat",
		N:       2,
		Quality: "high",
		Size:    "1024x1024",
	})
	if err != nil {
		t.Fatalf("GenerateImage() error = %v", err)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if payload.Model != "gpt-image-2" || payload.Prompt != "cat" || payload.N != 2 {
		t.Fatalf("payload = %+v", payload)
	}
	if len(result.Data) != 1 || !strings.HasPrefix(result.Data[0].URL, "data:image/png;base64,") {
		t.Fatalf("normalized result = %+v", result)
	}
	if result.Data[0].RevisedPrompt != "better" {
		t.Fatalf("revised prompt = %q", result.Data[0].RevisedPrompt)
	}
}

func TestClientEditImagePostsMultipartImage(t *testing.T) {
	var gotPath string
	var gotModel string
	var gotPrompt string
	var gotImageContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		gotModel = r.FormValue("model")
		gotPrompt = r.FormValue("prompt")
		files := r.MultipartForm.File["image[]"]
		if len(files) != 1 {
			t.Fatalf("image files = %d", len(files))
		}
		gotImageContentType = files[0].Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://example.invalid/out.png"}]}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "secret-key", time.Second, nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	result, err := client.EditImage(t.Context(), EditRequest{
		Model:            "gpt-image-2",
		Prompt:           "edit cat",
		N:                1,
		ImageBytes:       []byte("fake-png"),
		ImageFilename:    "source.png",
		ImageContentType: "image/png",
	})
	if err != nil {
		t.Fatalf("EditImage() error = %v", err)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotModel != "gpt-image-2" || gotPrompt != "edit cat" {
		t.Fatalf("multipart fields model=%q prompt=%q", gotModel, gotPrompt)
	}
	mediaType, _, err := mime.ParseMediaType(gotImageContentType)
	if err != nil {
		t.Fatalf("ParseMediaType(%q) error = %v", gotImageContentType, err)
	}
	if mediaType != "image/png" {
		t.Fatalf("image content type = %q", gotImageContentType)
	}
	if got := result.Data[0].URL; got != "https://example.invalid/out.png" {
		t.Fatalf("result URL = %q", got)
	}
}

func TestClientReturnsUpstreamErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded"}}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "secret-key", time.Second, nil)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.GenerateImage(t.Context(), GenerateRequest{Prompt: "cat"})
	if err == nil {
		t.Fatal("GenerateImage() expected error")
	}
	if !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("error = %v", err)
	}
}
