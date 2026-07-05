package invoice

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestDownloadTemporaryFileDoesNotRequireLoginAndExpires proves the fallback
// link is intentionally public, while the signed token still limits access.
func TestDownloadTemporaryFileDoesNotRequireLoginAndExpires(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	dataDir := t.TempDir()
	objectKey := "custom/invoices/2026/07/100.pdf"
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "custom", "invoices", "2026", "07"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "custom", "invoices", "2026", "07", "100.pdf"), []byte("%PDF-1.4\ninvoice"), 0644))

	files, err := NewFileStore(dataDir)
	require.NoError(t, err)
	service, err := NewServiceWithOptions(store, files, ServiceOptions{
		EmailSender:            &invoiceEmailSenderStub{},
		PublicDownloadBaseURL:  "https://api.example.com",
		PublicDownloadTokenKey: "test-secret",
		PublicDownloadTokenNow: func() time.Time { return now },
	})
	require.NoError(t, err)

	link, _, err := service.temporaryDownloadURL(Application{ID: 100, FileObjectKey: objectKey}, "https://api.example.com")
	require.NoError(t, err)
	token := strings.TrimPrefix(link, "https://api.example.com/api/v1/custom/invoice-downloads/")
	require.NotEqual(t, link, token)

	mock.ExpectQuery("FROM custom_invoice_applications").
		WithArgs(int64(100)).
		WillReturnRows(applicationRows(now).AddRow(
			int64(100), "INV20260705-K7Q9M2X4PA", int64(7), StatusIssued, InvoiceTypeEnterpriseVATNormal, int64(10),
			"Snapshot Inc", "TAX999", "invoice@example.com", "30.50000000", "CNY", 1,
			"FP-20260705", "已开票", "", objectKey, "issued.pdf", int64(16), int64(1), now, nil, nil, now, now,
		))
	mock.ExpectQuery("FROM custom_invoice_application_orders").
		WithArgs(int64(100)).
		WillReturnRows(applicationOrderRows().AddRow(
			int64(100), int64(501), int64(7), "30.50000000", "CNY", "order-501", payment.TypeStripe, payment.OrderStatusCompleted, now, now, now,
		))

	router := gin.New()
	router.GET("/api/v1/custom/invoice-downloads/:token", NewHandler(service).DownloadTemporaryFile)

	validReq := httptest.NewRequest(http.MethodGet, "/api/v1/custom/invoice-downloads/"+token, nil)
	validResp := httptest.NewRecorder()
	router.ServeHTTP(validResp, validReq)
	require.Equal(t, http.StatusOK, validResp.Code)
	require.Contains(t, validResp.Header().Get("Content-Disposition"), "issued.pdf")
	require.Contains(t, validResp.Body.String(), "%PDF-1.4")

	service.downloadTokenMaker.now = func() time.Time { return now.Add(25 * time.Hour) }
	expiredReq := httptest.NewRequest(http.MethodGet, "/api/v1/custom/invoice-downloads/"+token, nil)
	expiredResp := httptest.NewRecorder()
	router.ServeHTTP(expiredResp, expiredReq)
	require.Equal(t, http.StatusNotFound, expiredResp.Code)
	require.Contains(t, expiredResp.Body.String(), "下载链接无效或已过期")
}

func TestTestSendGeneratedNotificationDoesNotRequireApplicationRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	emailSender := &invoiceEmailSenderStub{}
	service := NewService(nil, nil, emailSender)

	router := gin.New()
	router.POST("/api/v1/admin/custom/invoice-test-email", NewHandler(service).TestSendGeneratedNotification)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/custom/invoice-test-email", bytes.NewBufferString(`{"receiver_email":"qa@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
	require.Len(t, emailSender.attachmentCalls, 1)
	require.Equal(t, "qa@example.com", emailSender.attachmentCalls[0].to)
	require.Empty(t, emailSender.linkCalls)
}

func TestTestSendGeneratedNotificationReturnsRedactedFailureCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	emailSender := &invoiceEmailSenderStub{attachmentErr: errors.New("smtp dial failed: smtp_password=secret")}
	service := NewService(nil, nil, emailSender)

	router := gin.New()
	router.POST("/api/v1/admin/custom/invoice-test-email", NewHandler(service).TestSendGeneratedNotification)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/custom/invoice-test-email", bytes.NewBufferString(`{"receiver_email":"qa@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusInternalServerError, resp.Code)
	require.Contains(t, resp.Body.String(), "开票通知发送失败")
	require.Contains(t, resp.Body.String(), "smtp dial failed")
	require.Contains(t, resp.Body.String(), "smtp_password=***")
	require.NotContains(t, resp.Body.String(), "secret")
}
