package invoice

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/gin-gonic/gin"
)

const loginExpiredMessage = "登录失效，请重新登录"

// Handler exposes custom invoice HTTP APIs.
type Handler struct {
	service *Service
}

// NewHandler creates an invoice handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ListTitles returns current user's reusable invoice titles.
func (h *Handler) ListTitles(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	titles, err := h.service.ListTitles(c.Request.Context(), subject.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, titles)
}

// CreateTitle creates a reusable enterprise invoice title.
func (h *Handler) CreateTitle(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	var req titleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	title, err := h.service.CreateTitle(c.Request.Context(), subject.UserID, req.toInput())
	if err != nil {
		writeError(c, err)
		return
	}
	response.Created(c, title)
}

// UpdateTitle updates one title owned by the current user.
func (h *Handler) UpdateTitle(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req titleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	title, err := h.service.UpdateTitle(c.Request.Context(), subject.UserID, id, req.toInput())
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, title)
}

// DeleteTitle soft-deletes one title.
func (h *Handler) DeleteTitle(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteTitle(c.Request.Context(), subject.UserID, id); err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// SetDefaultTitle marks one title as default.
func (h *Handler) SetDefaultTitle(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	title, err := h.service.SetDefaultTitle(c.Request.Context(), subject.UserID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, title)
}

// ListEligibleOrders returns current user's invoiceable recharge orders.
func (h *Handler) ListEligibleOrders(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	orders, err := h.service.ListEligibleOrders(c.Request.Context(), subject.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, gin.H{"items": orders})
}

// CreateApplication creates a pending invoice application.
func (h *Handler) CreateApplication(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	var req createApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	app, err := h.service.CreateApplication(c.Request.Context(), CreateApplicationInput{
		UserID:   subject.UserID,
		TitleID:  req.TitleID,
		OrderIDs: req.OrderIDs,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.Created(c, app)
}

// ListMyApplications returns current user's invoice applications.
func (h *Handler) ListMyApplications(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	apps, total, err := h.service.ListUserApplications(c.Request.Context(), ListApplicationsFilter{
		UserID:   subject.UserID,
		Status:   strings.TrimSpace(c.Query("status")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.Paginated(c, apps, int64(total), page, pageSize)
}

// GetMyApplication returns one user-owned application detail.
func (h *Handler) GetMyApplication(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	app, err := h.service.GetUserApplication(c.Request.Context(), id, subject.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, app)
}

// DownloadMyFile streams the current user's issued invoice PDF.
func (h *Handler) DownloadMyFile(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	app, fileStore, err := h.service.OpenUserInvoiceFile(c.Request.Context(), id, subject.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	serveFile(c, app, fileStore)
}

// ListAdminApplications returns invoice applications for admin review.
func (h *Handler) ListAdminApplications(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	userID, ok := parseOptionalInt64(c, "user_id")
	if !ok {
		return
	}
	apps, total, err := h.service.ListAdminApplications(c.Request.Context(), AdminListApplicationsFilter{
		UserID:   userID,
		Status:   strings.TrimSpace(c.Query("status")),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.Paginated(c, apps, int64(total), page, pageSize)
}

// GetAdminApplication returns one application detail for admins.
func (h *Handler) GetAdminApplication(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	app, err := h.service.GetAdminApplication(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, app)
}

// IssueApplication marks a pending application as issued with an uploaded PDF.
func (h *Handler) IssueApplication(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "请上传发票文件")
		return
	}
	app, err := h.service.IssueApplication(c.Request.Context(), id, IssueInput{
		AdminID:       subject.UserID,
		InvoiceNumber: c.PostForm("invoice_number"),
		AdminRemark:   c.PostForm("admin_remark"),
		PublicBaseURL: requestPublicBaseURL(c.Request),
	}, file)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, app)
}

// TestSendIssuedNotification sends the current issued invoice email again without status changes.
func (h *Handler) TestSendIssuedNotification(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.TestSendIssuedNotification(c.Request.Context(), id, requestPublicBaseURL(c.Request)); err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, gin.H{"sent": true})
}

// TestSendGeneratedNotification sends a generated invoice email to the requested recipient.
func (h *Handler) TestSendGeneratedNotification(c *gin.Context) {
	var req testEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	if err := h.service.TestSendGeneratedNotification(c.Request.Context(), TestEmailInput{
		ReceiverEmail: req.ReceiverEmail,
	}); err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, gin.H{"sent": true})
}

// RejectApplication rejects a pending application and releases its orders.
func (h *Handler) RejectApplication(c *gin.Context) {
	subject, ok := authSubject(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req rejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	app, err := h.service.RejectApplication(c.Request.Context(), id, RejectInput{AdminID: subject.UserID, Reason: req.Reason})
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, app)
}

// DownloadAdminFile streams an invoice PDF for admins.
func (h *Handler) DownloadAdminFile(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	app, fileStore, err := h.service.OpenAdminInvoiceFile(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	serveFile(c, app, fileStore)
}

// DownloadTemporaryFile streams an issued invoice PDF through a no-login expiring link.
func (h *Handler) DownloadTemporaryFile(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		response.NotFound(c, "下载链接无效或已过期")
		return
	}
	app, fileStore, err := h.service.OpenTemporaryInvoiceFile(c.Request.Context(), token)
	if err != nil {
		writeError(c, err)
		return
	}
	serveFile(c, app, fileStore)
}

type titleRequest struct {
	CompanyTitle  string `json:"company_title"`
	TaxNumber     string `json:"tax_number"`
	ReceiverEmail string `json:"receiver_email"`
	IsDefault     bool   `json:"is_default"`
}

func (r titleRequest) toInput() TitleInput {
	return TitleInput{
		CompanyTitle:  r.CompanyTitle,
		TaxNumber:     r.TaxNumber,
		ReceiverEmail: r.ReceiverEmail,
		IsDefault:     r.IsDefault,
	}
}

type createApplicationRequest struct {
	OrderIDs []int64 `json:"order_ids"`
	TitleID  int64   `json:"title_id"`
}

type rejectRequest struct {
	Reason string `json:"reason"`
}

type testEmailRequest struct {
	ReceiverEmail string `json:"receiver_email"`
}

func authSubject(c *gin.Context) (middleware.AuthSubject, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, loginExpiredMessage)
		return middleware.AuthSubject{}, false
	}
	return subject, true
}

func parseID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param(name)), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "请求参数无效")
		return 0, false
	}
	return id, true
}

func parseOptionalInt64(c *gin.Context, name string) (int64, bool) {
	raw := c.Query(name)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		response.BadRequest(c, "请求参数无效")
		return 0, false
	}
	return value, true
}

func writeError(c *gin.Context, err error) {
	status, message := classifyError(err)
	if status >= http.StatusInternalServerError {
		logInvoiceError(c, status, err)
	}
	if status >= 500 {
		response.InternalError(c, message)
		return
	}
	response.Error(c, status, message)
}

func logInvoiceError(c *gin.Context, status int, err error) {
	if err == nil {
		return
	}
	method := ""
	path := ""
	if c != nil && c.Request != nil {
		method = c.Request.Method
		path = c.Request.URL.Path
	}
	slog.Error("custom invoice request failed",
		"status", status,
		"method", method,
		"path", path,
		"error", logredact.RedactText(err.Error(), invoiceErrorRedactKeys()...),
	)
}

func serveFile(c *gin.Context, app Application, fileStore *FileStore) {
	file, path, err := fileStore.Open(app.FileObjectKey)
	if err != nil {
		writeError(c, errors.Join(ErrInvalidFile, err))
		return
	}
	_ = file.Close()
	name := strings.TrimSpace(app.FileOriginalName)
	if name == "" {
		name = "invoice.pdf"
	}
	c.Header("Content-Type", "application/pdf")
	c.FileAttachment(path, name)
}

func requestPublicBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	scheme := firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := firstForwardedValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if strings.TrimSpace(host) == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(scheme)) + "://" + strings.TrimSpace(host)
}

func firstForwardedValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}
