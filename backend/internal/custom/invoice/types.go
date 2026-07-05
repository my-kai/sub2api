package invoice

import (
	"context"
	"errors"
	"time"

	coreservice "github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	// StatusPending marks an application waiting for admin review.
	StatusPending = "pending"
	// StatusIssued marks an application that has been issued with a PDF file.
	StatusIssued = "issued"
	// StatusRejected marks an application rejected by an admin; its orders are reusable.
	StatusRejected = "rejected"

	// InvoiceTypeEnterpriseVATNormal is the only supported invoice type for now.
	InvoiceTypeEnterpriseVATNormal = "enterprise_vat_normal"

	// MaxPDFSizeBytes keeps uploaded electronic invoice files bounded.
	MaxPDFSizeBytes int64 = 10 * 1024 * 1024

	// invoicePDFContentType is the MIME type used for issued invoice attachments.
	invoicePDFContentType = "application/pdf"
)

var (
	ErrInvalidInput        = errors.New("invoice input is invalid")
	ErrTitleNotFound       = errors.New("invoice title not found")
	ErrApplicationNotFound = errors.New("invoice application not found")
	ErrOrderNotEligible    = errors.New("invoice order is not eligible")
	ErrOrderOccupied       = errors.New("invoice order is already occupied")
	ErrInvalidStatus       = errors.New("invoice status is invalid")
	ErrInvalidFile         = errors.New("invoice file is invalid")
	ErrNotificationMissing = errors.New("invoice notification email sender is missing")
	ErrNotificationFailed  = errors.New("invoice notification email failed")
	ErrPublicLinkInvalid   = errors.New("invoice public download link is invalid")
	ErrPublicLinkExpired   = errors.New("invoice public download link is expired")
	ErrPublicLinkMissing   = errors.New("invoice public download link config is missing")
)

// EmailSender is the small subset of the main email service required by invoice completion.
type EmailSender interface {
	RenderNotificationEmail(ctx context.Context, input coreservice.NotificationEmailSendInput) (coreservice.NotificationEmailPreview, error)
	SendEmail(ctx context.Context, to, subject, body string) error
	SendEmailWithAttachment(ctx context.Context, to, subject, body string, attachment coreservice.EmailAttachment) error
}

// Title stores a user's reusable enterprise invoice title.
type Title struct {
	ID            int64      `json:"id"`
	UserID        int64      `json:"user_id"`
	CompanyTitle  string     `json:"company_title"`
	TaxNumber     string     `json:"tax_number"`
	ReceiverEmail string     `json:"receiver_email"`
	IsDefault     bool       `json:"is_default"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TitleInput is the validated shape for title create/update operations.
type TitleInput struct {
	CompanyTitle  string
	TaxNumber     string
	ReceiverEmail string
	IsDefault     bool
}

// EligibleOrder is a recharge order that can be attached to a new invoice request.
type EligibleOrder struct {
	ID          int64      `json:"id"`
	OutTradeNo  string     `json:"out_trade_no"`
	Amount      string     `json:"amount"`
	PayAmount   string     `json:"pay_amount"`
	Currency    string     `json:"currency"`
	PaymentType string     `json:"payment_type"`
	Status      string     `json:"status"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ApplicationOrder is the immutable order snapshot bound to an application.
type ApplicationOrder struct {
	ApplicationID int64      `json:"application_id"`
	OrderID       int64      `json:"order_id"`
	UserID        int64      `json:"user_id"`
	Amount        string     `json:"amount"`
	Currency      string     `json:"currency"`
	OutTradeNo    string     `json:"out_trade_no,omitempty"`
	PaymentType   string     `json:"payment_type,omitempty"`
	Status        string     `json:"status,omitempty"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Application stores an invoice request and its current review result.
type Application struct {
	ID               int64              `json:"id"`
	ApplicationNo    string             `json:"application_no"`
	UserID           int64              `json:"user_id"`
	Status           string             `json:"status"`
	InvoiceType      string             `json:"invoice_type"`
	TitleID          *int64             `json:"title_id,omitempty"`
	CompanyTitle     string             `json:"company_title"`
	TaxNumber        string             `json:"tax_number"`
	ReceiverEmail    string             `json:"receiver_email"`
	TotalAmount      string             `json:"total_amount"`
	Currency         string             `json:"currency"`
	OrderCount       int                `json:"order_count"`
	InvoiceNumber    string             `json:"invoice_number"`
	AdminRemark      string             `json:"admin_remark"`
	RejectReason     string             `json:"reject_reason"`
	FileObjectKey    string             `json:"-"`
	FileOriginalName string             `json:"file_original_name,omitempty"`
	FileSize         int64              `json:"file_size,omitempty"`
	IssuedBy         *int64             `json:"issued_by,omitempty"`
	IssuedAt         *time.Time         `json:"issued_at,omitempty"`
	RejectedBy       *int64             `json:"rejected_by,omitempty"`
	RejectedAt       *time.Time         `json:"rejected_at,omitempty"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
	Orders           []ApplicationOrder `json:"orders,omitempty"`
}

// CreateApplicationInput contains the user-selected title and recharge orders.
type CreateApplicationInput struct {
	UserID   int64
	TitleID  int64
	OrderIDs []int64
}

// ListApplicationsFilter controls paginated application queries.
type ListApplicationsFilter struct {
	UserID   int64
	Status   string
	Page     int
	PageSize int
}

// AdminListApplicationsFilter controls admin invoice queries.
type AdminListApplicationsFilter struct {
	UserID   int64
	Status   string
	Page     int
	PageSize int
}

// IssueInput contains the metadata and file location persisted after PDF upload.
type IssueInput struct {
	AdminID          int64
	InvoiceNumber    string
	AdminRemark      string
	PublicBaseURL    string
	FileObjectKey    string
	FileOriginalName string
	FileSize         int64
}

// TestEmailInput contains the recipient for a generated invoice notification test.
type TestEmailInput struct {
	ReceiverEmail string
}

// RejectInput contains the admin rejection reason.
type RejectInput struct {
	AdminID int64
	Reason  string
}

// StoredFile identifies a persisted invoice PDF.
type StoredFile struct {
	ObjectKey    string
	OriginalName string
	Size         int64
	Path         string
}
