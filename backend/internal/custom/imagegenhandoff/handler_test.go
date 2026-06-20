package imagegenhandoff

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type stubUserReader struct {
	user *service.User
	err  error
}

func (r stubUserReader) GetByID(context.Context, int64) (*service.User, error) {
	return r.user, r.err
}

func TestLoginCodeAndExchange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewMemoryCodeStore(5 * time.Minute)
	handler := NewHandler(Config{
		BaseURL:        "https://image-gen.example.invalid/app/",
		ExchangeSecret: "shared-secret",
		CodeTTLSeconds: 300,
	}, store, stubUserReader{user: &service.User{
		ID:       42,
		Username: "alice",
		Email:    "alice@example.com",
		Role:     service.RoleAdmin,
		Status:   service.StatusActive,
	}})

	router := gin.New()
	router.POST("/login-code", func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		handler.LoginCode(c)
	})
	router.POST("/exchange", handler.Exchange)

	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/login-code", nil))
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}

	var loginResp struct {
		Code int `json:"code"`
		Data struct {
			RedirectURL string    `json:"redirect_url"`
			ExpiresAt   time.Time `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !strings.HasPrefix(loginResp.Data.RedirectURL, "https://image-gen.example.invalid/app/auth/callback?code=once_") {
		t.Fatalf("redirect_url = %q", loginResp.Data.RedirectURL)
	}
	code := strings.TrimPrefix(loginResp.Data.RedirectURL, "https://image-gen.example.invalid/app/auth/callback?code=")

	exchangeBody := bytes.NewBufferString(`{"code":"` + code + `"}`)
	exchangeReq := httptest.NewRequest(http.MethodPost, "/exchange", exchangeBody)
	exchangeReq.Header.Set("Content-Type", "application/json")
	exchangeReq.Header.Set(exchangeSecretHeader, "shared-secret")
	exchangeRec := httptest.NewRecorder()
	router.ServeHTTP(exchangeRec, exchangeReq)
	if exchangeRec.Code != http.StatusOK {
		t.Fatalf("exchange status = %d, body = %s", exchangeRec.Code, exchangeRec.Body.String())
	}

	var exchangeResp struct {
		User Identity `json:"user"`
	}
	if err := json.Unmarshal(exchangeRec.Body.Bytes(), &exchangeResp); err != nil {
		t.Fatalf("decode exchange response: %v", err)
	}
	if exchangeResp.User.ExternalUserID != "42" || !exchangeResp.User.IsAdmin {
		t.Fatalf("unexpected user = %+v", exchangeResp.User)
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/exchange", bytes.NewBufferString(`{"code":"`+code+`"}`))
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.Header.Set(exchangeSecretHeader, "shared-secret")
	replayRec := httptest.NewRecorder()
	router.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, want 401", replayRec.Code)
	}
	if strings.Contains(replayRec.Body.String(), "shared-secret") || strings.Contains(replayRec.Body.String(), "once_") {
		t.Fatalf("exchange failure leaked internal data: %s", replayRec.Body.String())
	}
}

func TestExchangeRejectsBadSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(Config{ExchangeSecret: "shared-secret"}, NewMemoryCodeStore(5*time.Minute), nil)
	router := gin.New()
	router.POST("/exchange", handler.Exchange)

	req := httptest.NewRequest(http.MethodPost, "/exchange", bytes.NewBufferString(`{"code":"once_x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(exchangeSecretHeader, "wrong-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "wrong-secret") || strings.Contains(rec.Body.String(), "shared-secret") {
		t.Fatalf("bad secret leaked in response: %s", rec.Body.String())
	}
}
