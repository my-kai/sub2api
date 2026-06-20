package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/imagequeue"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"

	"github.com/gin-gonic/gin"
)

func TestOpenAIImageGenerationsReturnsOpenAIResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &openAIQueueServiceStub{
		userForKeyUser: runtime.UserProfile{ID: 7},
		createJob: imagequeue.Job{
			ID:        21,
			UserID:    7,
			CreatedAt: time.Unix(111, 0).UTC(),
		},
		waitJob: imagequeue.Job{
			ID:        21,
			UserID:    7,
			Status:    imagequeue.JobStatusCompleted,
			CreatedAt: time.Unix(111, 0).UTC(),
			Result: &chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{
				URL: "https://img.local/1.png",
			}}},
		},
	}
	handler := &ImageGenerationHandler{queueService: service}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/custom/openai/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2","prompt":"draw","n":1}`))
	context.Request.Header.Set("Authorization", "Bearer sk-img-valid")
	context.Request.Header.Set("Content-Type", "application/json")

	handler.OpenAIImageGenerations(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var response chatgpt2api.ImageGenerationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Created != 111 || len(response.Data) != 1 || response.Data[0].URL != "https://img.local/1.png" {
		t.Fatalf("response = %+v", response)
	}
	if service.userForKeyToken != "sk-img-valid" {
		t.Fatalf("token = %q", service.userForKeyToken)
	}
	if service.createInput.SessionID != 0 || service.createInput.Prompt != "draw" {
		t.Fatalf("create input = %+v", service.createInput)
	}
	if service.createUser.ID != 7 || service.waitUser.ID != 7 || service.waitID != 21 {
		t.Fatalf("user/wait = %+v %+v %d", service.createUser, service.waitUser, service.waitID)
	}
}

func TestOpenAIImageGenerationsRejectsInvalidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &openAIQueueServiceStub{userForKeyErr: imagequeue.ErrAPIKeyNotFound}
	handler := &ImageGenerationHandler{queueService: service}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/custom/openai/v1/images/generations", strings.NewReader(`{"prompt":"draw"}`))
	context.Request.Header.Set("Authorization", "Bearer sk-img-deleted")
	context.Request.Header.Set("Content-Type", "application/json")

	handler.OpenAIImageGenerations(context)

	assertOpenAIErrorCode(t, recorder, http.StatusUnauthorized, "invalid_api_key")
	if service.createCalled {
		t.Fatal("CreateOpenAITask should not be called for invalid key")
	}
}

func TestOpenAIImageGenerationsTimeoutDoesNotCancelTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &openAIQueueServiceStub{
		userForKeyUser: runtime.UserProfile{ID: 7},
		createJob:      imagequeue.Job{ID: 21, UserID: 7},
		waitErr:        imagequeue.ErrTaskWaitTimeout,
	}
	handler := &ImageGenerationHandler{queueService: service}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/custom/openai/v1/images/generations", strings.NewReader(`{"prompt":"draw"}`))
	context.Request.Header.Set("Authorization", "Bearer sk-img-valid")
	context.Request.Header.Set("Content-Type", "application/json")

	handler.OpenAIImageGenerations(context)

	assertOpenAIErrorCode(t, recorder, http.StatusGatewayTimeout, "timeout")
	if service.cancelCalled {
		t.Fatal("timeout must not cancel background task")
	}
}

func TestOpenAIImageGenerationsReturnsFailedTaskAsOpenAIError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &openAIQueueServiceStub{
		userForKeyUser: runtime.UserProfile{ID: 7},
		createJob:      imagequeue.Job{ID: 21, UserID: 7},
		waitJob: imagequeue.Job{
			ID:           21,
			UserID:       7,
			Status:       imagequeue.JobStatusFailed,
			ErrorMessage: "upstream exploded",
		},
	}
	handler := &ImageGenerationHandler{queueService: service}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/custom/openai/v1/images/generations", strings.NewReader(`{"prompt":"draw"}`))
	context.Request.Header.Set("Authorization", "Bearer sk-img-valid")
	context.Request.Header.Set("Content-Type", "application/json")

	handler.OpenAIImageGenerations(context)

	assertOpenAIErrorCode(t, recorder, http.StatusBadGateway, "image_generation_failed")
	if !strings.Contains(recorder.Body.String(), "Image generation task failed. Please try again later.") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
	for _, leaked := range []string{"upstream exploded", "chatgpt2api", "auth key"} {
		if strings.Contains(strings.ToLower(recorder.Body.String()), strings.ToLower(leaked)) {
			t.Fatalf("body leaked upstream detail %q: %s", leaked, recorder.Body.String())
		}
	}
}

func TestOpenAIImageEditsStoresUploadedImageBeforeWaiting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &openAIQueueServiceStub{
		userForKeyUser: runtime.UserProfile{ID: 7},
		createJob:      imagequeue.Job{ID: 21, UserID: 7},
		waitJob: imagequeue.Job{
			ID:     21,
			UserID: 7,
			Status: imagequeue.JobStatusCompleted,
			Result: &chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{
				URL: "https://img.local/edit.png",
			}}},
		},
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	mustWriteField(t, writer, "prompt", "edit")
	part, err := writer.CreateFormFile("image", "source.png")
	if err != nil {
		t.Fatalf("create image field: %v", err)
	}
	if _, err := part.Write([]byte("image-bytes")); err != nil {
		t.Fatalf("write image field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	handler := &ImageGenerationHandler{queueService: service}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/custom/openai/v1/images/edits", &body)
	context.Request.Header.Set("Authorization", "Bearer sk-img-valid")
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())

	handler.OpenAIImageEdits(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if string(service.createInput.SourceImageBytes) != "image-bytes" || service.createInput.SourceImageFilename != "source.png" {
		t.Fatalf("uploaded image input = %+v", service.createInput)
	}
}

func assertOpenAIErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error.Code != code {
		t.Fatalf("error code = %q body = %s", payload.Error.Code, recorder.Body.String())
	}
}

type openAIQueueServiceStub struct {
	userForKeyToken string
	userForKeyUser  runtime.UserProfile
	userForKeyErr   error
	createCalled    bool
	createUser      runtime.UserProfile
	createInput     imagequeue.CreateJobInput
	createJob       imagequeue.Job
	createErr       error
	waitUser        runtime.UserProfile
	waitID          int64
	waitJob         imagequeue.Job
	waitErr         error
	taskJob         imagequeue.Job
	cancelCalled    bool
}

func (s *openAIQueueServiceStub) CreateSession(context.Context, runtime.UserProfile, imagequeue.CreateSessionInput) (imagequeue.Session, error) {
	return imagequeue.Session{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) Sessions(context.Context, runtime.UserProfile) ([]imagequeue.Session, error) {
	return nil, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) UpdateSession(context.Context, runtime.UserProfile, int64, imagequeue.UpdateSessionInput) (imagequeue.Session, error) {
	return imagequeue.Session{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) DeleteSession(context.Context, runtime.UserProfile, int64) error {
	return errors.New("not implemented")
}

func (s *openAIQueueServiceStub) SetCurrentImage(context.Context, runtime.UserProfile, int64, imagequeue.SetCurrentImageInput) (imagequeue.Session, error) {
	return imagequeue.Session{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) ResetCurrentImage(context.Context, runtime.UserProfile, int64) (imagequeue.Session, error) {
	return imagequeue.Session{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) SessionTasks(context.Context, runtime.UserProfile, int64, imagequeue.PageRequest) (imagequeue.PageResult[imagequeue.Job], error) {
	return imagequeue.PageResult[imagequeue.Job]{Items: []imagequeue.Job{}}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) SubscribeTaskEvents(ctx context.Context) (<-chan imagequeue.TaskEvent, func()) {
	ch := make(chan imagequeue.TaskEvent)
	close(ch)
	return ch, func() {}
}

func (s *openAIQueueServiceStub) MyImages(context.Context, runtime.UserProfile, imagequeue.PageRequest) (imagequeue.PageResult[imagequeue.MyImage], error) {
	return imagequeue.PageResult[imagequeue.MyImage]{Items: []imagequeue.MyImage{}}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) CreateTask(context.Context, runtime.UserProfile, imagequeue.CreateJobInput) (imagequeue.Job, error) {
	return imagequeue.Job{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) RetryTask(context.Context, runtime.UserProfile, int64) (imagequeue.Job, error) {
	return imagequeue.Job{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) Task(context.Context, runtime.UserProfile, int64) (imagequeue.Job, error) {
	return s.taskJob, nil
}

func (s *openAIQueueServiceStub) CancelTask(context.Context, runtime.UserProfile, int64) (imagequeue.Job, error) {
	s.cancelCalled = true
	return imagequeue.Job{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) Config(context.Context) (imagequeue.Config, error) {
	return imagequeue.Config{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) PublicStatus(context.Context) (imagequeue.PublicStatus, error) {
	return imagequeue.PublicStatus{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) QuotePrice(context.Context, imagequeue.PriceQuoteInput) (imagequeue.PriceQuote, error) {
	return imagequeue.PriceQuote{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) APIKeys(context.Context, runtime.UserProfile) ([]imagequeue.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) CreateAPIKey(context.Context, runtime.UserProfile, imagequeue.CreateAPIKeyInput) (imagequeue.APIKey, error) {
	return imagequeue.APIKey{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) DeleteAPIKey(context.Context, runtime.UserProfile, int64) error {
	return errors.New("not implemented")
}

func (s *openAIQueueServiceStub) UserForAPIKey(_ context.Context, plaintext string) (runtime.UserProfile, imagequeue.APIKey, error) {
	s.userForKeyToken = plaintext
	if s.userForKeyErr != nil {
		return runtime.UserProfile{}, imagequeue.APIKey{}, s.userForKeyErr
	}
	return s.userForKeyUser, imagequeue.APIKey{ID: 1, UserID: s.userForKeyUser.ID}, nil
}

func (s *openAIQueueServiceStub) CreateOpenAITask(_ context.Context, user runtime.UserProfile, input imagequeue.CreateJobInput) (imagequeue.Job, error) {
	s.createCalled = true
	s.createUser = user
	s.createInput = input
	return s.createJob, s.createErr
}

func (s *openAIQueueServiceStub) WaitTaskTerminal(_ context.Context, user runtime.UserProfile, id int64, timeout time.Duration) (imagequeue.Job, error) {
	s.waitUser = user
	s.waitID = id
	if timeout != openAIImageWaitTimeout {
		return imagequeue.Job{}, errors.New("unexpected timeout")
	}
	return s.waitJob, s.waitErr
}

func (s *openAIQueueServiceStub) UpdateConfig(context.Context, imagequeue.ConfigInput, runtime.UserProfile) (imagequeue.Config, error) {
	return imagequeue.Config{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) UserLimits(context.Context) ([]imagequeue.UserLimit, error) {
	return nil, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) UpsertUserLimit(context.Context, int64, imagequeue.UserLimitInput, imagequeue.UserLimitSnapshot) (imagequeue.UserLimit, error) {
	return imagequeue.UserLimit{}, errors.New("not implemented")
}

func (s *openAIQueueServiceStub) DeleteUserLimit(context.Context, int64) error {
	return errors.New("not implemented")
}

type staticUserResolver struct {
	user runtime.UserProfile
}

func (r staticUserResolver) RequireUser(*gin.Context) (runtime.UserProfile, error) {
	return r.user, nil
}

func (r staticUserResolver) RequireAdmin(*gin.Context) (runtime.UserProfile, error) {
	return r.user, nil
}

func (r staticUserResolver) OptionalUser(*gin.Context) (runtime.UserProfile, bool) {
	return r.user, r.user.ID > 0
}
