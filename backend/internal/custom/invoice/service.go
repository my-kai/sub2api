package invoice

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const invoiceIssueRollbackTimeout = 10 * time.Second
const publicDownloadTokenTTL = 24 * time.Hour

const invoiceTestPDFText = "Invoice notification test PDF"

// Service owns invoice business validation and status transitions.
type Service struct {
	store              *Store
	files              *FileStore
	emailSender        EmailSender
	publicBaseURL      string
	downloadTokenTTL   time.Duration
	downloadTokenMaker *publicDownloadTokenSigner
}

// ServiceOptions contains optional invoice service dependencies.
type ServiceOptions struct {
	EmailSender            EmailSender
	PublicDownloadBaseURL  string
	PublicDownloadTokenTTL time.Duration
	PublicDownloadTokenKey string
	PublicDownloadTokenNow func() time.Time
}

// NewService builds an invoice service from its persistence dependencies.
func NewService(store *Store, files *FileStore, emailSenders ...EmailSender) *Service {
	var emailSender EmailSender
	if len(emailSenders) > 0 {
		emailSender = emailSenders[0]
	}
	return &Service{store: store, files: files, emailSender: emailSender, downloadTokenTTL: publicDownloadTokenTTL}
}

// NewServiceWithOptions builds an invoice service with email and temporary-link support.
func NewServiceWithOptions(store *Store, files *FileStore, opts ServiceOptions) (*Service, error) {
	tokenTTL := opts.PublicDownloadTokenTTL
	if tokenTTL <= 0 {
		tokenTTL = publicDownloadTokenTTL
	}
	var signer *publicDownloadTokenSigner
	if strings.TrimSpace(opts.PublicDownloadTokenKey) != "" {
		var err error
		signer, err = newPublicDownloadTokenSigner(opts.PublicDownloadTokenKey)
		if err != nil {
			return nil, err
		}
		if opts.PublicDownloadTokenNow != nil {
			signer.now = opts.PublicDownloadTokenNow
		}
	}
	if opts.EmailSender != nil && signer == nil {
		return nil, ErrPublicLinkMissing
	}
	return &Service{
		store:              store,
		files:              files,
		emailSender:        opts.EmailSender,
		publicBaseURL:      strings.TrimRight(strings.TrimSpace(opts.PublicDownloadBaseURL), "/"),
		downloadTokenTTL:   tokenTTL,
		downloadTokenMaker: signer,
	}, nil
}

// ListTitles returns the current user's invoice titles.
func (s *Service) ListTitles(ctx context.Context, userID int64) ([]Title, error) {
	if s == nil || s.store == nil || userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListTitles(ctx, userID)
}

// CreateTitle validates and creates a reusable enterprise invoice title.
func (s *Service) CreateTitle(ctx context.Context, userID int64, input TitleInput) (Title, error) {
	if err := validateTitleInput(&input); err != nil {
		return Title{}, err
	}
	return s.store.CreateTitle(ctx, userID, input)
}

// UpdateTitle validates and updates one title owned by the user.
func (s *Service) UpdateTitle(ctx context.Context, userID, titleID int64, input TitleInput) (Title, error) {
	if err := validateTitleInput(&input); err != nil {
		return Title{}, err
	}
	return s.store.UpdateTitle(ctx, userID, titleID, input)
}

// DeleteTitle soft-deletes one title owned by the user.
func (s *Service) DeleteTitle(ctx context.Context, userID, titleID int64) error {
	if s == nil || s.store == nil || userID <= 0 || titleID <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteTitle(ctx, userID, titleID)
}

// SetDefaultTitle marks one title as the user's default title.
func (s *Service) SetDefaultTitle(ctx context.Context, userID, titleID int64) (Title, error) {
	if s == nil || s.store == nil || userID <= 0 || titleID <= 0 {
		return Title{}, ErrInvalidInput
	}
	return s.store.SetDefaultTitle(ctx, userID, titleID)
}

// ListEligibleOrders returns recharge orders that can be selected for a new application.
func (s *Service) ListEligibleOrders(ctx context.Context, userID int64) ([]EligibleOrder, error) {
	if s == nil || s.store == nil || userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListEligibleOrders(ctx, userID)
}

// CreateApplication creates one pending invoice request from selected orders and title.
func (s *Service) CreateApplication(ctx context.Context, input CreateApplicationInput) (Application, error) {
	if s == nil || s.store == nil || input.UserID <= 0 || input.TitleID <= 0 {
		return Application{}, ErrInvalidInput
	}
	ids, err := validateUniquePositiveIDs(input.OrderIDs)
	if err != nil {
		return Application{}, err
	}
	input.OrderIDs = ids
	return s.store.CreateApplication(ctx, input)
}

// ListUserApplications returns paginated applications for a user.
func (s *Service) ListUserApplications(ctx context.Context, filter ListApplicationsFilter) ([]Application, int, error) {
	if s == nil || s.store == nil || filter.UserID <= 0 {
		return nil, 0, ErrInvalidInput
	}
	if filter.Status != "" && !validStatus(filter.Status) {
		return nil, 0, ErrInvalidInput
	}
	return s.store.ListUserApplications(ctx, filter)
}

// ListAdminApplications returns paginated applications for admins.
func (s *Service) ListAdminApplications(ctx context.Context, filter AdminListApplicationsFilter) ([]Application, int, error) {
	if s == nil || s.store == nil {
		return nil, 0, ErrInvalidInput
	}
	if filter.Status != "" && !validStatus(filter.Status) {
		return nil, 0, ErrInvalidInput
	}
	return s.store.ListAdminApplications(ctx, filter)
}

// GetUserApplication loads an application owned by the current user.
func (s *Service) GetUserApplication(ctx context.Context, id, userID int64) (Application, error) {
	if s == nil || s.store == nil || id <= 0 || userID <= 0 {
		return Application{}, ErrInvalidInput
	}
	return s.store.GetApplication(ctx, id, userID)
}

// GetAdminApplication loads an application for admin review.
func (s *Service) GetAdminApplication(ctx context.Context, id int64) (Application, error) {
	if s == nil || s.store == nil || id <= 0 {
		return Application{}, ErrInvalidInput
	}
	return s.store.GetApplication(ctx, id, 0)
}

// IssueApplication validates PDF metadata, stores the file and marks an application as issued.
func (s *Service) IssueApplication(ctx context.Context, id int64, input IssueInput, file *multipart.FileHeader) (Application, error) {
	if s == nil || s.store == nil || s.files == nil || id <= 0 || input.AdminID <= 0 {
		return Application{}, ErrInvalidInput
	}
	input.InvoiceNumber = strings.TrimSpace(input.InvoiceNumber)
	input.AdminRemark = strings.TrimSpace(input.AdminRemark)
	if input.InvoiceNumber == "" || input.AdminRemark == "" || file == nil {
		return Application{}, ErrInvalidInput
	}
	if s.emailSender == nil {
		return Application{}, ErrNotificationMissing
	}
	stored, err := s.files.SavePDF(ctx, id, file)
	if err != nil {
		return Application{}, err
	}
	input.FileObjectKey = stored.ObjectKey
	input.FileOriginalName = stored.OriginalName
	input.FileSize = stored.Size
	app, err := s.store.IssueApplication(ctx, id, input)
	if err != nil {
		// The file belongs to this status update; remove it if the database update fails.
		s.files.Remove(stored.ObjectKey)
		return Application{}, err
	}
	if err := s.sendIssuedNotification(ctx, app, stored.Path, input.PublicBaseURL); err != nil {
		// Completion requires a delivered email with the invoice PDF. Roll the
		// status back so an admin can retry after fixing SMTP or recipient issues.
		revertCtx, cancel := context.WithTimeout(context.Background(), invoiceIssueRollbackTimeout)
		defer cancel()
		revertErr := s.store.RevertIssuedApplication(revertCtx, id, input)
		s.files.Remove(stored.ObjectKey)
		if revertErr != nil {
			return Application{}, errors.Join(ErrNotificationFailed, err, fmt.Errorf("revert issued invoice application: %w", revertErr))
		}
		return Application{}, errors.Join(ErrNotificationFailed, err)
	}
	return app, nil
}

// TestSendIssuedNotification resends the issued invoice email without changing application state.
func (s *Service) TestSendIssuedNotification(ctx context.Context, id int64, publicBaseURL string) error {
	if s == nil || s.store == nil || s.files == nil || id <= 0 {
		return ErrInvalidInput
	}
	if s.emailSender == nil {
		return ErrNotificationMissing
	}
	app, err := s.store.GetApplication(ctx, id, 0)
	if err != nil {
		return err
	}
	if app.Status != StatusIssued || strings.TrimSpace(app.FileObjectKey) == "" {
		return ErrInvalidStatus
	}
	file, path, err := s.files.Open(app.FileObjectKey)
	if err != nil {
		return errors.Join(ErrInvalidFile, err)
	}
	_ = file.Close()
	if err := s.sendIssuedNotification(ctx, app, path, publicBaseURL); err != nil {
		return errors.Join(ErrNotificationFailed, err)
	}
	return nil
}

// TestSendGeneratedNotification sends an invoice notification with generated test data.
//
// The test path intentionally does not read or write invoice applications, so
// admins can verify template rendering and attachment delivery before any real
// application exists.
func (s *Service) TestSendGeneratedNotification(ctx context.Context, input TestEmailInput) error {
	if s == nil {
		return ErrInvalidInput
	}
	if s.emailSender == nil {
		return ErrNotificationMissing
	}
	receiver, err := normalizeTestReceiverEmail(input.ReceiverEmail)
	if err != nil {
		return err
	}
	pdf := generatedInvoiceTestPDF()
	app := generatedTestApplication(receiver, time.Now().UTC(), int64(len(pdf)))
	rendered, err := s.renderIssuedNotification(ctx, coreservice.NotificationEmailEventInvoiceIssuedAttachment, app, nil)
	if err != nil {
		return errors.Join(ErrNotificationFailed, err)
	}
	if err := s.sendInvoiceAttachment(ctx, receiver, rendered, pdf, invoiceAttachmentName(app)); err != nil {
		return errors.Join(ErrNotificationFailed, err)
	}
	return nil
}

// RejectApplication rejects a pending application and releases its orders.
func (s *Service) RejectApplication(ctx context.Context, id int64, input RejectInput) (Application, error) {
	if s == nil || s.store == nil || id <= 0 || input.AdminID <= 0 {
		return Application{}, ErrInvalidInput
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" {
		return Application{}, ErrInvalidInput
	}
	return s.store.RejectApplication(ctx, id, input)
}

// OpenUserInvoiceFile opens a PDF for a user-owned issued application.
func (s *Service) OpenUserInvoiceFile(ctx context.Context, id, userID int64) (Application, *FileStore, error) {
	app, err := s.GetUserApplication(ctx, id, userID)
	if err != nil {
		return Application{}, nil, err
	}
	if app.Status != StatusIssued || strings.TrimSpace(app.FileObjectKey) == "" {
		return Application{}, nil, ErrInvalidStatus
	}
	return app, s.files, nil
}

// OpenAdminInvoiceFile opens a PDF for an admin.
func (s *Service) OpenAdminInvoiceFile(ctx context.Context, id int64) (Application, *FileStore, error) {
	app, err := s.GetAdminApplication(ctx, id)
	if err != nil {
		return Application{}, nil, err
	}
	if strings.TrimSpace(app.FileObjectKey) == "" {
		return Application{}, nil, ErrInvalidStatus
	}
	return app, s.files, nil
}

// OpenTemporaryInvoiceFile opens a no-login invoice PDF when the temporary token is valid.
func (s *Service) OpenTemporaryInvoiceFile(ctx context.Context, token string) (Application, *FileStore, error) {
	if s == nil || s.store == nil || s.files == nil || s.downloadTokenMaker == nil {
		return Application{}, nil, ErrPublicLinkMissing
	}
	payload, err := s.downloadTokenMaker.verify(token)
	if err != nil {
		return Application{}, nil, linkTokenError(err)
	}
	app, err := s.store.GetApplication(ctx, payload.ApplicationID, 0)
	if err != nil {
		return Application{}, nil, err
	}
	if app.Status != StatusIssued || strings.TrimSpace(app.FileObjectKey) == "" {
		return Application{}, nil, ErrPublicLinkInvalid
	}
	if !strings.EqualFold(hashObjectKey(app.FileObjectKey), payload.ObjectKeyHash) {
		return Application{}, nil, ErrPublicLinkInvalid
	}
	return app, s.files, nil
}

func validateTitleInput(input *TitleInput) error {
	if input == nil {
		return ErrInvalidInput
	}
	input.CompanyTitle = strings.TrimSpace(input.CompanyTitle)
	input.TaxNumber = strings.TrimSpace(input.TaxNumber)
	input.ReceiverEmail = strings.TrimSpace(input.ReceiverEmail)
	if input.CompanyTitle == "" || input.TaxNumber == "" || input.ReceiverEmail == "" {
		return ErrInvalidInput
	}
	if _, err := mail.ParseAddress(input.ReceiverEmail); err != nil {
		return ErrInvalidInput
	}
	return nil
}

func (s *Service) sendIssuedNotification(ctx context.Context, app Application, path string, inputBaseURL string) error {
	if s.emailSender == nil {
		return ErrNotificationMissing
	}
	recipient := strings.TrimSpace(app.ReceiverEmail)
	if recipient == "" {
		return ErrNotificationFailed
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read issued invoice PDF: %w", err)
	}
	if len(content) == 0 {
		return ErrInvalidFile
	}
	rendered, err := s.renderIssuedNotification(ctx, coreservice.NotificationEmailEventInvoiceIssuedAttachment, app, nil)
	if err != nil {
		return err
	}
	if err := s.sendInvoiceAttachment(ctx, recipient, rendered, content, invoiceAttachmentName(app)); err != nil {
		if linkErr := s.sendIssuedNotificationWithLink(ctx, app, inputBaseURL, err); linkErr != nil {
			return linkErr
		}
	}
	return nil
}

func (s *Service) sendIssuedNotificationWithLink(ctx context.Context, app Application, inputBaseURL string, attachmentErr error) error {
	link, expiresAt, err := s.temporaryDownloadURL(app, inputBaseURL)
	if err != nil {
		return errors.Join(ErrPublicLinkMissing, attachmentErr, err)
	}
	rendered, err := s.renderIssuedNotification(ctx, coreservice.NotificationEmailEventInvoiceIssuedLink, app, map[string]string{
		"download_url":    link,
		"link_expires_at": expiresAt.Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return errors.Join(ErrNotificationFailed, attachmentErr, err)
	}
	if err := s.emailSender.SendEmail(ctx, app.ReceiverEmail, rendered.Subject, rendered.HTML); err != nil {
		return errors.Join(ErrNotificationFailed, attachmentErr, fmt.Errorf("send issued invoice link email: %w", err))
	}
	return nil
}

func (s *Service) sendInvoiceAttachment(ctx context.Context, recipient string, rendered coreservice.NotificationEmailPreview, content []byte, attachmentName string) error {
	if len(content) == 0 {
		return ErrInvalidFile
	}
	return s.emailSender.SendEmailWithAttachment(ctx, recipient, rendered.Subject, rendered.HTML, coreservice.EmailAttachment{
		Filename:    attachmentName,
		ContentType: invoicePDFContentType,
		Data:        content,
	})
}

func (s *Service) temporaryDownloadURL(app Application, inputBaseURL string) (string, time.Time, error) {
	if s == nil || s.downloadTokenMaker == nil {
		return "", time.Time{}, ErrPublicLinkMissing
	}
	baseURL := strings.TrimRight(strings.TrimSpace(inputBaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/")
	}
	if err := validatePublicBaseURL(baseURL); err != nil {
		return "", time.Time{}, err
	}
	token, expiresAt, err := s.downloadTokenMaker.issue(app, s.downloadTokenTTL)
	if err != nil {
		return "", time.Time{}, err
	}
	return baseURL + "/api/v1/custom/invoice-downloads/" + url.PathEscape(token), expiresAt, nil
}

func (s *Service) renderIssuedNotification(ctx context.Context, event string, app Application, extra map[string]string) (coreservice.NotificationEmailPreview, error) {
	if s == nil || s.emailSender == nil {
		return coreservice.NotificationEmailPreview{}, ErrNotificationMissing
	}
	variables := map[string]string{
		"application_no":   strings.TrimSpace(app.ApplicationNo),
		"invoice_number":   strings.TrimSpace(app.InvoiceNumber),
		"company_title":    strings.TrimSpace(app.CompanyTitle),
		"invoice_amount":   strings.TrimSpace(app.TotalAmount),
		"invoice_currency": strings.TrimSpace(app.Currency),
	}
	for key, value := range extra {
		variables[key] = value
	}
	rendered, err := s.emailSender.RenderNotificationEmail(ctx, coreservice.NotificationEmailSendInput{
		Event:          event,
		RecipientEmail: strings.TrimSpace(app.ReceiverEmail),
		RecipientName:  strings.TrimSpace(app.CompanyTitle),
		UserID:         app.UserID,
		SourceType:     "invoice_application",
		SourceID:       fmt.Sprintf("%d", app.ID),
		Variables:      variables,
	})
	if err != nil {
		return coreservice.NotificationEmailPreview{}, fmt.Errorf("render issued invoice email template: %w", err)
	}
	return rendered, nil
}

func normalizeTestReceiverEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrInvalidInput
	}
	address, err := mail.ParseAddress(trimmed)
	if err != nil || strings.TrimSpace(address.Address) == "" {
		return "", ErrInvalidInput
	}
	return strings.TrimSpace(address.Address), nil
}

func generatedTestApplication(receiverEmail string, now time.Time, fileSize int64) Application {
	stamp := now.Format("20060102150405")
	return Application{
		ApplicationNo:    "INV-TEST-" + stamp,
		UserID:           0,
		Status:           StatusIssued,
		InvoiceType:      InvoiceTypeEnterpriseVATNormal,
		CompanyTitle:     "测试开票抬头",
		TaxNumber:        "TEST1234567890",
		ReceiverEmail:    receiverEmail,
		TotalAmount:      "100.00",
		Currency:         "CNY",
		OrderCount:       1,
		InvoiceNumber:    "TEST-FP-" + stamp,
		AdminRemark:      "测试发件",
		FileOriginalName: "invoice-test-" + stamp + ".pdf",
		FileSize:         fileSize,
		IssuedAt:         &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// generatedInvoiceTestPDF creates a small valid PDF attachment for SMTP capability tests.
func generatedInvoiceTestPDF() []byte {
	stream := fmt.Sprintf("BT /F1 12 Tf 40 90 Td (%s) Tj ET", invoiceTestPDFText)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var builder strings.Builder
	builder.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		objectNumber := index + 1
		offsets[objectNumber] = builder.Len()
		fmt.Fprintf(&builder, "%d 0 obj\n%s\nendobj\n", objectNumber, object)
	}
	xrefOffset := builder.Len()
	fmt.Fprintf(&builder, "xref\n0 %d\n0000000000 65535 f\n", len(objects)+1)
	for objectNumber := 1; objectNumber <= len(objects); objectNumber++ {
		fmt.Fprintf(&builder, "%010d 00000 n\n", offsets[objectNumber])
	}
	fmt.Fprintf(&builder, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return []byte(builder.String())
}

func invoiceAttachmentName(app Application) string {
	no := strings.TrimSpace(app.ApplicationNo)
	if no == "" {
		no = fmt.Sprintf("%d", app.ID)
	}
	return "invoice-" + no + ".pdf"
}

func validatePublicBaseURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || !parsed.IsAbs() {
		return ErrPublicLinkMissing
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrPublicLinkMissing
	}
	if strings.TrimSpace(parsed.Host) == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ErrPublicLinkMissing
	}
	return nil
}

func validateUniquePositiveIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, ErrInvalidInput
	}
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalidInput
		}
		if _, ok := seen[id]; ok {
			return nil, ErrInvalidInput
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func validStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case StatusPending, StatusIssued, StatusRejected:
		return true
	default:
		return false
	}
}

func classifyError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrPublicLinkInvalid), errors.Is(err, ErrPublicLinkExpired):
		return 404, "下载链接无效或已过期"
	case errors.Is(err, ErrPublicLinkMissing):
		return 500, invoiceFailureMessage("临时下载链接生成失败", err, ErrPublicLinkMissing)
	case errors.Is(err, ErrNotificationMissing):
		return 500, invoiceFailureMessage("开票通知邮箱未配置", err, ErrNotificationMissing)
	case errors.Is(err, ErrNotificationFailed):
		return 500, invoiceFailureMessage("开票通知发送失败", err, ErrNotificationFailed)
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrInvalidFile):
		return 400, "请求参数无效"
	case errors.Is(err, ErrOrderNotEligible):
		return 400, "订单不可开票"
	case errors.Is(err, ErrOrderOccupied):
		return 409, "订单已被开票申请占用"
	case errors.Is(err, ErrTitleNotFound):
		return 404, "发票抬头不存在"
	case errors.Is(err, ErrApplicationNotFound):
		return 404, "开票申请不存在"
	default:
		return 500, "开票服务暂不可用"
	}
}

func invoiceFailureMessage(prefix string, err error, sentinels ...error) string {
	detail := invoiceErrorDetail(err, sentinels...)
	if detail == "" {
		return prefix
	}
	return prefix + "：" + detail
}

func invoiceErrorDetail(err error, sentinels ...error) string {
	if err == nil {
		return ""
	}
	detail := logredact.RedactText(err.Error(), invoiceErrorRedactKeys()...)
	for _, sentinel := range sentinels {
		if sentinel == nil {
			continue
		}
		detail = strings.ReplaceAll(detail, sentinel.Error(), "")
	}
	replacer := strings.NewReplacer("\r\n", "；", "\n", "；", "\t", " ")
	detail = replacer.Replace(detail)
	for strings.Contains(detail, "；；") {
		detail = strings.ReplaceAll(detail, "；；", "；")
	}
	detail = strings.Trim(detail, " \t\r\n:：;；")
	if len([]rune(detail)) > 500 {
		runes := []rune(detail)
		detail = string(runes[:500]) + "..."
	}
	return detail
}

func invoiceErrorRedactKeys() []string {
	return []string{
		"api_key",
		"apikey",
		"secret",
		"smtp_password",
		"smtp_pass",
		"smtp_secret",
		"token",
	}
}
