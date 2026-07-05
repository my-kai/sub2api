package invoice

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateTitleDefaultClearsPrevious(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_invoice_titles")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO custom_invoice_titles")).
		WithArgs(int64(7), "Acme Inc", "TAX123", "billing@example.com", true).
		WillReturnRows(titleRows().AddRow(int64(10), int64(7), "Acme Inc", "TAX123", "billing@example.com", true, nil, now, now))
	mock.ExpectCommit()

	title, err := store.CreateTitle(t.Context(), 7, TitleInput{
		CompanyTitle:  "Acme Inc",
		TaxNumber:     "TAX123",
		ReceiverEmail: "billing@example.com",
		IsDefault:     true,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), title.ID)
	require.True(t, title.IsDefault)
}

func TestStoreSetDefaultTitleClearsOtherDefaults(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM custom_invoice_titles")).
		WithArgs(int64(10), int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_invoice_titles")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_invoice_titles")).
		WithArgs(int64(10), int64(7)).
		WillReturnRows(titleRows().AddRow(int64(10), int64(7), "Acme Inc", "TAX123", "billing@example.com", true, nil, now, now))
	mock.ExpectCommit()

	title, err := store.SetDefaultTitle(t.Context(), 7, 10)
	require.NoError(t, err)
	require.Equal(t, int64(10), title.ID)
	require.True(t, title.IsDefault)
}

func TestStoreDeleteTitleSoftDeletes(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_invoice_titles")).
		WithArgs(int64(10), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, store.DeleteTitle(t.Context(), 7, 10))
}

func TestStoreCreateApplicationBindsMultipleRechargeOrdersWithTitleSnapshot(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	paidAt := now.Add(-time.Hour)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, company_title")).
		WithArgs(int64(10), int64(7)).
		WillReturnRows(titleRows().AddRow(int64(10), int64(7), "Snapshot Inc", "TAX999", "invoice@example.com", true, nil, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM payment_orders")).
		WithArgs(int64(7), sqlmock.AnyArg(), defaultCurrency, payment.OrderTypeBalance, sqlmock.AnyArg()).
		WillReturnRows(eligibleOrderRows().
			AddRow(int64(501), "order-501", "10.10", "10.10", "CNY", payment.TypeStripe, payment.OrderStatusCompleted, paidAt, now, now).
			AddRow(int64(502), "order-502", "20.40", "20.40", "CNY", payment.TypeStripe, payment.OrderStatusCompleted, paidAt, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_application_orders")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"order_id"}))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO custom_invoice_applications")).
		WithArgs(
			sqlmock.AnyArg(),
			int64(7),
			StatusPending,
			InvoiceTypeEnterpriseVATNormal,
			int64(10),
			"Snapshot Inc",
			"TAX999",
			"invoice@example.com",
			"30.50000000",
			"CNY",
			2,
		).
		WillReturnRows(applicationRows(now).AddRow(
			int64(100), "INV20260705-K7Q9M2X4PA", int64(7), StatusPending, InvoiceTypeEnterpriseVATNormal, int64(10),
			"Snapshot Inc", "TAX999", "invoice@example.com", "30.50000000", "CNY", 2,
			"", "", "", "", "", int64(0), nil, nil, nil, nil, now, now,
		))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO custom_invoice_application_orders")).
		WithArgs(int64(100), int64(501), int64(7), "10.10", "CNY").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO custom_invoice_application_orders")).
		WithArgs(int64(100), int64(502), int64(7), "20.40", "CNY").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_applications")).
		WithArgs(int64(100), int64(7)).
		WillReturnRows(applicationRows(now).AddRow(
			int64(100), "INV20260705-K7Q9M2X4PA", int64(7), StatusPending, InvoiceTypeEnterpriseVATNormal, int64(10),
			"Snapshot Inc", "TAX999", "invoice@example.com", "30.50000000", "CNY", 2,
			"", "", "", "", "", int64(0), nil, nil, nil, nil, now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_application_orders")).
		WithArgs(int64(100)).
		WillReturnRows(applicationOrderRows().
			AddRow(int64(100), int64(501), int64(7), "10.10", "CNY", "order-501", payment.TypeStripe, payment.OrderStatusCompleted, paidAt, now, now).
			AddRow(int64(100), int64(502), int64(7), "20.40", "CNY", "order-502", payment.TypeStripe, payment.OrderStatusCompleted, paidAt, now, now))

	app, err := store.CreateApplication(t.Context(), CreateApplicationInput{
		UserID:   7,
		TitleID:  10,
		OrderIDs: []int64{501, 502},
	})
	require.NoError(t, err)
	require.Equal(t, int64(100), app.ID)
	require.Equal(t, "INV20260705-K7Q9M2X4PA", app.ApplicationNo)
	require.Equal(t, "Snapshot Inc", app.CompanyTitle)
	require.Equal(t, "TAX999", app.TaxNumber)
	require.Equal(t, "30.50000000", app.TotalAmount)
	require.Len(t, app.Orders, 2)
}

func TestStoreCreateApplicationRejectsIneligibleOrder(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, company_title")).
		WithArgs(int64(10), int64(7)).
		WillReturnRows(titleRows().AddRow(int64(10), int64(7), "Snapshot Inc", "TAX999", "invoice@example.com", true, nil, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM payment_orders")).
		WithArgs(int64(7), sqlmock.AnyArg(), defaultCurrency, payment.OrderTypeBalance, sqlmock.AnyArg()).
		WillReturnRows(eligibleOrderRows().
			AddRow(int64(501), "order-501", "10.10", "10.10", "CNY", payment.TypeStripe, payment.OrderStatusCompleted, now, now, now))
	mock.ExpectRollback()

	_, err := store.CreateApplication(t.Context(), CreateApplicationInput{
		UserID:   7,
		TitleID:  10,
		OrderIDs: []int64{501, 502},
	})
	require.True(t, errors.Is(err, ErrOrderNotEligible))
}

func TestStoreCreateApplicationRejectsOccupiedOrder(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, company_title")).
		WithArgs(int64(10), int64(7)).
		WillReturnRows(titleRows().AddRow(int64(10), int64(7), "Snapshot Inc", "TAX999", "invoice@example.com", true, nil, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM payment_orders")).
		WithArgs(int64(7), sqlmock.AnyArg(), defaultCurrency, payment.OrderTypeBalance, sqlmock.AnyArg()).
		WillReturnRows(eligibleOrderRows().
			AddRow(int64(501), "order-501", "10.10", "10.10", "CNY", payment.TypeStripe, payment.OrderStatusCompleted, now, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_application_orders")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"order_id"}).AddRow(int64(501)))
	mock.ExpectRollback()

	_, err := store.CreateApplication(t.Context(), CreateApplicationInput{
		UserID:   7,
		TitleID:  10,
		OrderIDs: []int64{501},
	})
	require.True(t, errors.Is(err, ErrOrderOccupied))
}

func TestStoreRejectApplicationOnlyUpdatesPending(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_invoice_applications")).
		WithArgs(int64(100), StatusRejected, "资料不完整", int64(1), StatusPending).
		WillReturnError(sql.ErrNoRows)

	_, err := store.RejectApplication(t.Context(), 100, RejectInput{AdminID: 1, Reason: "资料不完整"})
	require.True(t, errors.Is(err, ErrInvalidStatus))
}

func TestServiceRejectAndIssueRequireAdminInputs(t *testing.T) {
	store, _, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	files, err := NewFileStore(t.TempDir())
	require.NoError(t, err)
	service := NewService(store, files)

	_, err = service.CreateTitle(t.Context(), 7, TitleInput{
		CompanyTitle:  "Acme Inc",
		TaxNumber:     "TAX123",
		ReceiverEmail: "not-an-email",
	})
	require.True(t, errors.Is(err, ErrInvalidInput))

	_, err = service.RejectApplication(t.Context(), 100, RejectInput{AdminID: 1, Reason: "   "})
	require.True(t, errors.Is(err, ErrInvalidInput))

	_, err = service.IssueApplication(t.Context(), 100, IssueInput{
		AdminID:       1,
		InvoiceNumber: "INV-1",
		AdminRemark:   "已开票",
	}, nil)
	require.True(t, errors.Is(err, ErrInvalidInput))
}

func TestServiceIssueApplicationSendsPDFNotificationAttachment(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	files, err := NewFileStore(t.TempDir())
	require.NoError(t, err)
	emailSender := &invoiceEmailSenderStub{}
	service := NewService(store, files, emailSender)
	file := invoicePDFFileHeader(t, "issued.pdf", "%PDF-1.4\ninvoice-body")

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_invoice_applications")).
		WithArgs(int64(100), StatusIssued, "FP-20260705", "已开票", sqlmock.AnyArg(), "issued.pdf", file.Size, int64(1), StatusPending).
		WillReturnRows(applicationRows(now).AddRow(
			int64(100), "INV20260705-K7Q9M2X4PA", int64(7), StatusIssued, InvoiceTypeEnterpriseVATNormal, int64(10),
			"Snapshot Inc", "TAX999", "invoice@example.com", "30.50000000", "CNY", 1,
			"FP-20260705", "已开票", "", "custom/invoices/2026/07/100.pdf", "issued.pdf", file.Size, int64(1), now, nil, nil, now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_application_orders")).
		WithArgs(int64(100)).
		WillReturnRows(applicationOrderRows().AddRow(
			int64(100), int64(501), int64(7), "30.50000000", "CNY", "order-501", payment.TypeStripe, payment.OrderStatusCompleted, now, now, now,
		))

	app, err := service.IssueApplication(t.Context(), 100, IssueInput{
		AdminID:       1,
		InvoiceNumber: "FP-20260705",
		AdminRemark:   "已开票",
	}, file)
	require.NoError(t, err)
	require.Equal(t, StatusIssued, app.Status)
	require.Len(t, emailSender.attachmentCalls, 1)
	require.Empty(t, emailSender.linkCalls)
	require.Len(t, emailSender.renderCalls, 1)
	require.Equal(t, coreservice.NotificationEmailEventInvoiceIssuedAttachment, emailSender.renderCalls[0].Event)
	call := emailSender.attachmentCalls[0]
	require.Equal(t, "invoice@example.com", call.to)
	require.Contains(t, call.subject, "INV20260705-K7Q9M2X4PA")
	require.Equal(t, "invoice-INV20260705-K7Q9M2X4PA.pdf", call.attachment.Filename)
	require.Equal(t, invoicePDFContentType, call.attachment.ContentType)
	require.True(t, bytes.HasPrefix(call.attachment.Data, []byte("%PDF-")))
	require.Contains(t, call.body, "FP-20260705")
}

func TestServiceIssueApplicationRollsBackWhenNotificationFails(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	dataDir := t.TempDir()
	files, err := NewFileStore(dataDir)
	require.NoError(t, err)
	emailSender := &invoiceEmailSenderStub{
		attachmentErr: errors.New("attachment unsupported"),
		sendErr:       errors.New("smtp unavailable"),
	}
	service, err := NewServiceWithOptions(store, files, ServiceOptions{
		EmailSender:            emailSender,
		PublicDownloadBaseURL:  "https://api.example.com",
		PublicDownloadTokenKey: "test-secret",
	})
	require.NoError(t, err)
	file := invoicePDFFileHeader(t, "issued.pdf", "%PDF-1.4\ninvoice-body")

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_invoice_applications")).
		WithArgs(int64(100), StatusIssued, "FP-20260705", "已开票", sqlmock.AnyArg(), "issued.pdf", file.Size, int64(1), StatusPending).
		WillReturnRows(applicationRows(now).AddRow(
			int64(100), "INV20260705-K7Q9M2X4PA", int64(7), StatusIssued, InvoiceTypeEnterpriseVATNormal, int64(10),
			"Snapshot Inc", "TAX999", "invoice@example.com", "30.50000000", "CNY", 1,
			"FP-20260705", "已开票", "", "custom/invoices/2026/07/100.pdf", "issued.pdf", file.Size, int64(1), now, nil, nil, now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_application_orders")).
		WithArgs(int64(100)).
		WillReturnRows(applicationOrderRows().AddRow(
			int64(100), int64(501), int64(7), "30.50000000", "CNY", "order-501", payment.TypeStripe, payment.OrderStatusCompleted, now, now, now,
		))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_invoice_applications")).
		WithArgs(int64(100), StatusPending, StatusIssued, sqlmock.AnyArg(), "FP-20260705").
		WillReturnResult(sqlmock.NewResult(0, 1))

	_, err = service.IssueApplication(t.Context(), 100, IssueInput{
		AdminID:       1,
		InvoiceNumber: "FP-20260705",
		AdminRemark:   "已开票",
	}, file)
	require.ErrorIs(t, err, ErrNotificationFailed)
	require.Len(t, emailSender.renderCalls, 2)
	require.Equal(t, coreservice.NotificationEmailEventInvoiceIssuedAttachment, emailSender.renderCalls[0].Event)
	require.Equal(t, coreservice.NotificationEmailEventInvoiceIssuedLink, emailSender.renderCalls[1].Event)
	require.Len(t, emailSender.attachmentCalls, 1)
	require.Len(t, emailSender.linkCalls, 1)
	requireInvoicePDFCount(t, dataDir, 0)
}

func TestServiceIssueApplicationFallsBackToTemporaryDownloadLink(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	files, err := NewFileStore(t.TempDir())
	require.NoError(t, err)
	emailSender := &invoiceEmailSenderStub{attachmentErr: errors.New("attachment unsupported")}
	service, err := NewServiceWithOptions(store, files, ServiceOptions{
		EmailSender:            emailSender,
		PublicDownloadBaseURL:  "https://api.example.com",
		PublicDownloadTokenKey: "test-secret",
	})
	require.NoError(t, err)
	file := invoicePDFFileHeader(t, "issued.pdf", "%PDF-1.4\ninvoice-body")

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_invoice_applications")).
		WithArgs(int64(100), StatusIssued, "FP-20260705", "已开票", sqlmock.AnyArg(), "issued.pdf", file.Size, int64(1), StatusPending).
		WillReturnRows(applicationRows(now).AddRow(
			int64(100), "INV20260705-K7Q9M2X4PA", int64(7), StatusIssued, InvoiceTypeEnterpriseVATNormal, int64(10),
			"Snapshot Inc", "TAX999", "invoice@example.com", "30.50000000", "CNY", 1,
			"FP-20260705", "已开票", "", "custom/invoices/2026/07/100.pdf", "issued.pdf", file.Size, int64(1), now, nil, nil, now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_application_orders")).
		WithArgs(int64(100)).
		WillReturnRows(applicationOrderRows().AddRow(
			int64(100), int64(501), int64(7), "30.50000000", "CNY", "order-501", payment.TypeStripe, payment.OrderStatusCompleted, now, now, now,
		))

	app, err := service.IssueApplication(t.Context(), 100, IssueInput{
		AdminID:       1,
		InvoiceNumber: "FP-20260705",
		AdminRemark:   "已开票",
	}, file)
	require.NoError(t, err)
	require.Equal(t, StatusIssued, app.Status)
	require.Len(t, emailSender.renderCalls, 2)
	require.Equal(t, coreservice.NotificationEmailEventInvoiceIssuedAttachment, emailSender.renderCalls[0].Event)
	require.Equal(t, coreservice.NotificationEmailEventInvoiceIssuedLink, emailSender.renderCalls[1].Event)
	require.Len(t, emailSender.attachmentCalls, 1)
	require.Len(t, emailSender.linkCalls, 1)
	require.Contains(t, emailSender.linkCalls[0].body, "https://api.example.com/api/v1/custom/invoice-downloads/")
	require.Contains(t, emailSender.linkCalls[0].body, "链接有效期")
}

func TestServiceTestSendIssuedNotificationResendsWithoutStatusChange(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	dataDir := t.TempDir()
	objectKey := "custom/invoices/2026/07/100.pdf"
	absPath := filepath.Join(dataDir, filepath.FromSlash(objectKey))
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0755))
	require.NoError(t, os.WriteFile(absPath, []byte("%PDF-1.4\ninvoice-body"), 0644))

	files, err := NewFileStore(dataDir)
	require.NoError(t, err)
	emailSender := &invoiceEmailSenderStub{}
	service := NewService(store, files, emailSender)

	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_applications")).
		WithArgs(int64(100)).
		WillReturnRows(applicationRows(now).AddRow(
			int64(100), "INV20260705-K7Q9M2X4PA", int64(7), StatusIssued, InvoiceTypeEnterpriseVATNormal, int64(10),
			"Snapshot Inc", "TAX999", "invoice@example.com", "30.50000000", "CNY", 1,
			"FP-20260705", "已开票", "", objectKey, "issued.pdf", int64(21), int64(1), now, nil, nil, now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_application_orders")).
		WithArgs(int64(100)).
		WillReturnRows(applicationOrderRows().AddRow(
			int64(100), int64(501), int64(7), "30.50000000", "CNY", "order-501", payment.TypeStripe, payment.OrderStatusCompleted, now, now, now,
		))

	err = service.TestSendIssuedNotification(t.Context(), 100, "https://api.example.com")
	require.NoError(t, err)
	require.Len(t, emailSender.renderCalls, 1)
	require.Equal(t, coreservice.NotificationEmailEventInvoiceIssuedAttachment, emailSender.renderCalls[0].Event)
	require.Len(t, emailSender.attachmentCalls, 1)
	require.Empty(t, emailSender.linkCalls)
	require.Contains(t, emailSender.attachmentCalls[0].subject, "INV20260705-K7Q9M2X4PA")
	require.True(t, bytes.HasPrefix(emailSender.attachmentCalls[0].attachment.Data, []byte("%PDF-")))
}

func TestServiceTestSendIssuedNotificationRequiresIssuedApplication(t *testing.T) {
	store, mock, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	files, err := NewFileStore(t.TempDir())
	require.NoError(t, err)
	emailSender := &invoiceEmailSenderStub{}
	service := NewService(store, files, emailSender)

	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_applications")).
		WithArgs(int64(100)).
		WillReturnRows(applicationRows(now).AddRow(
			int64(100), "INV20260705-K7Q9M2X4PA", int64(7), StatusPending, InvoiceTypeEnterpriseVATNormal, int64(10),
			"Snapshot Inc", "TAX999", "invoice@example.com", "30.50000000", "CNY", 1,
			"", "", "", "", "", int64(0), nil, nil, nil, nil, now, now,
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_invoice_application_orders")).
		WithArgs(int64(100)).
		WillReturnRows(applicationOrderRows().AddRow(
			int64(100), int64(501), int64(7), "30.50000000", "CNY", "order-501", payment.TypeStripe, payment.OrderStatusCompleted, now, now, now,
		))

	err = service.TestSendIssuedNotification(t.Context(), 100, "https://api.example.com")
	require.ErrorIs(t, err, ErrInvalidStatus)
	require.Empty(t, emailSender.renderCalls)
	require.Empty(t, emailSender.attachmentCalls)
	require.Empty(t, emailSender.linkCalls)
}

func TestServiceTestSendGeneratedNotificationUsesGeneratedInfoWithoutApplicationRecord(t *testing.T) {
	emailSender := &invoiceEmailSenderStub{}
	service := NewService(nil, nil, emailSender)

	err := service.TestSendGeneratedNotification(t.Context(), TestEmailInput{ReceiverEmail: "qa@example.com"})
	require.NoError(t, err)
	require.Len(t, emailSender.renderCalls, 1)
	require.Equal(t, coreservice.NotificationEmailEventInvoiceIssuedAttachment, emailSender.renderCalls[0].Event)
	require.Equal(t, "qa@example.com", emailSender.renderCalls[0].RecipientEmail)
	require.Contains(t, emailSender.renderCalls[0].Variables["application_no"], "INV-TEST-")
	require.Contains(t, emailSender.renderCalls[0].Variables["invoice_number"], "TEST-FP-")
	require.Len(t, emailSender.attachmentCalls, 1)
	require.Equal(t, "qa@example.com", emailSender.attachmentCalls[0].to)
	require.Equal(t, invoicePDFContentType, emailSender.attachmentCalls[0].attachment.ContentType)
	require.True(t, bytes.HasPrefix(emailSender.attachmentCalls[0].attachment.Data, []byte("%PDF-")))
	require.Empty(t, emailSender.linkCalls)
}

func TestServiceTestSendGeneratedNotificationRejectsInvalidReceiver(t *testing.T) {
	emailSender := &invoiceEmailSenderStub{}
	service := NewService(nil, nil, emailSender)

	err := service.TestSendGeneratedNotification(t.Context(), TestEmailInput{ReceiverEmail: "not-an-email"})
	require.ErrorIs(t, err, ErrInvalidInput)
	require.Empty(t, emailSender.renderCalls)
	require.Empty(t, emailSender.attachmentCalls)
	require.Empty(t, emailSender.linkCalls)
}

func TestClassifyErrorIncludesRedactedNotificationCause(t *testing.T) {
	err := errors.Join(ErrNotificationFailed, errors.New("smtp auth failed: smtp_password=secret access_token=abc"))

	status, message := classifyError(err)
	require.Equal(t, 500, status)
	require.Contains(t, message, "开票通知发送失败")
	require.Contains(t, message, "smtp auth failed")
	require.Contains(t, message, "smtp_password=***")
	require.Contains(t, message, "access_token=***")
	require.NotContains(t, message, "secret")
	require.NotContains(t, message, "abc")
}

func TestPublicDownloadTokenExpiresAndValidatesObjectKey(t *testing.T) {
	now := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	signer, err := newPublicDownloadTokenSigner("test-secret")
	require.NoError(t, err)
	signer.now = func() time.Time { return now }

	token, _, err := signer.issue(Application{ID: 100, FileObjectKey: "custom/invoices/2026/07/100.pdf"}, time.Hour)
	require.NoError(t, err)
	payload, err := signer.verify(token)
	require.NoError(t, err)
	require.Equal(t, int64(100), payload.ApplicationID)
	require.Equal(t, hashObjectKey("custom/invoices/2026/07/100.pdf"), payload.ObjectKeyHash)

	signer.now = func() time.Time { return now.Add(2 * time.Hour) }
	_, err = signer.verify(token)
	require.ErrorIs(t, err, ErrPublicLinkExpired)
	require.False(t, strings.EqualFold(hashObjectKey("other.pdf"), payload.ObjectKeyHash))
}

func TestServiceCreateApplicationRejectsInvalidOrderIDs(t *testing.T) {
	store, _, cleanup := newInvoiceStoreMock(t)
	defer cleanup()

	service := NewService(store, nil)

	_, err := service.CreateApplication(t.Context(), CreateApplicationInput{
		UserID:   7,
		TitleID:  10,
		OrderIDs: []int64{501, 501},
	})
	require.True(t, errors.Is(err, ErrInvalidInput))

	_, err = service.CreateApplication(t.Context(), CreateApplicationInput{
		UserID:   7,
		TitleID:  10,
		OrderIDs: []int64{501, 0},
	})
	require.True(t, errors.Is(err, ErrInvalidInput))
}

type invoiceEmailCall struct {
	to         string
	subject    string
	body       string
	attachment coreservice.EmailAttachment
}

type invoiceEmailSenderStub struct {
	attachmentErr   error
	sendErr         error
	renderErr       error
	renderCalls     []coreservice.NotificationEmailSendInput
	attachmentCalls []invoiceEmailCall
	linkCalls       []invoiceEmailCall
}

func (s *invoiceEmailSenderStub) RenderNotificationEmail(_ context.Context, input coreservice.NotificationEmailSendInput) (coreservice.NotificationEmailPreview, error) {
	s.renderCalls = append(s.renderCalls, input)
	if s.renderErr != nil {
		return coreservice.NotificationEmailPreview{}, s.renderErr
	}
	subject := "开票完成通知：" + input.Variables["application_no"]
	body := "<p>" + input.Variables["invoice_number"] + "</p>"
	if input.Event == coreservice.NotificationEmailEventInvoiceIssuedLink {
		body += `<p><a class="button" href="` + input.Variables["download_url"] + `">下载发票</a></p><p>链接有效期：` + input.Variables["link_expires_at"] + `</p>`
	}
	return coreservice.NotificationEmailPreview{Subject: subject, HTML: body}, nil
}

func (s *invoiceEmailSenderStub) SendEmail(_ context.Context, to, subject, body string) error {
	s.linkCalls = append(s.linkCalls, invoiceEmailCall{to: to, subject: subject, body: body})
	return s.sendErr
}

func (s *invoiceEmailSenderStub) SendEmailWithAttachment(_ context.Context, to, subject, body string, attachment coreservice.EmailAttachment) error {
	copied := make([]byte, len(attachment.Data))
	copy(copied, attachment.Data)
	attachment.Data = copied
	s.attachmentCalls = append(s.attachmentCalls, invoiceEmailCall{to: to, subject: subject, body: body, attachment: attachment})
	return s.attachmentErr
}

func invoicePDFFileHeader(t *testing.T, name string, content string) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", "/invoice", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(MaxPDFSizeBytes))
	files := req.MultipartForm.File["file"]
	require.Len(t, files, 1)
	return files[0]
}

func requireInvoicePDFCount(t *testing.T, dataDir string, want int) {
	t.Helper()
	count := 0
	err := filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".pdf" {
			count++
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, want, count)
}

func TestInvoiceOrderRules(t *testing.T) {
	total, currency, err := sumApplicationOrders([]EligibleOrder{
		{PayAmount: "0.10", Currency: "CNY"},
		{PayAmount: "0.20", Currency: "CNY"},
	})
	require.NoError(t, err)
	require.Equal(t, "0.30000000", total)
	require.Equal(t, "CNY", currency)

	_, _, err = sumApplicationOrders([]EligibleOrder{
		{PayAmount: "1.00", Currency: "CNY"},
		{PayAmount: "1.00", Currency: "USD"},
	})
	require.True(t, errors.Is(err, ErrOrderNotEligible))
	require.NotContains(t, occupyingStatuses(), StatusRejected)
	require.Equal(t, []string{payment.OrderStatusCompleted}, invoiceableRechargeStatuses())
	require.NotContains(t, invoiceableRechargeStatuses(), payment.OrderStatusPaid)
	require.NotContains(t, invoiceableRechargeStatuses(), payment.OrderStatusRecharging)
}

func TestGenerateApplicationNoUsesDateAndUnorderedSuffix(t *testing.T) {
	no, err := generateApplicationNo(time.Date(2026, 7, 4, 16, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Regexp(t, `^INV20260705-[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{10}$`, no)
}

func newInvoiceStoreMock(t *testing.T) (*Store, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	store, err := NewStore(db)
	require.NoError(t, err)
	return store, mock, func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func titleRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "user_id", "company_title", "tax_number", "receiver_email",
		"is_default", "deleted_at", "created_at", "updated_at",
	})
}

func eligibleOrderRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "out_trade_no", "amount", "pay_amount", "currency",
		"payment_type", "status", "paid_at", "completed_at", "created_at",
	})
}

func applicationRows(_ time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "application_no", "user_id", "status", "invoice_type", "title_id",
		"company_title", "tax_number", "receiver_email", "total_amount",
		"currency", "order_count", "invoice_number", "admin_remark",
		"reject_reason", "file_object_key", "file_original_name", "file_size",
		"issued_by", "issued_at", "rejected_by", "rejected_at", "created_at", "updated_at",
	})
}

func applicationOrderRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"application_id", "order_id", "user_id", "amount", "currency",
		"out_trade_no", "payment_type", "status", "paid_at", "completed_at", "created_at",
	})
}
