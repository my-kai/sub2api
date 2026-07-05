package invoice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/lib/pq"
)

const defaultCurrency = "CNY"
const createApplicationNoMaxAttempts = 5

// Store owns SQL persistence for custom invoice applications.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// NewStore creates a PostgreSQL-backed invoice store.
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("sql db is required")
	}
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

// ListTitles returns non-deleted titles for one user, with default titles first.
func (s *Store) ListTitles(ctx context.Context, userID int64) ([]Title, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, company_title, tax_number, receiver_email, is_default,
		       deleted_at, created_at, updated_at
		FROM custom_invoice_titles
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY is_default DESC, created_at DESC, id DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list invoice titles: %w", err)
	}
	defer rows.Close()
	return scanTitles(rows)
}

// CreateTitle creates one enterprise invoice title and optionally makes it default.
func (s *Store) CreateTitle(ctx context.Context, userID int64, input TitleInput) (Title, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return Title{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Title{}, fmt.Errorf("begin invoice title transaction: %w", err)
	}
	committed := false
	defer rollbackUnlessCommitted(tx, &committed)

	if input.IsDefault {
		if _, err := tx.ExecContext(ctx, `
			UPDATE custom_invoice_titles
			SET is_default = FALSE, updated_at = NOW()
			WHERE user_id = $1 AND deleted_at IS NULL
		`, userID); err != nil {
			return Title{}, fmt.Errorf("clear default invoice titles: %w", err)
		}
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO custom_invoice_titles (
			user_id, company_title, tax_number, receiver_email, is_default, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id, user_id, company_title, tax_number, receiver_email, is_default,
		          deleted_at, created_at, updated_at
	`, userID, input.CompanyTitle, input.TaxNumber, input.ReceiverEmail, input.IsDefault)
	title, err := scanTitle(row)
	if err != nil {
		return Title{}, fmt.Errorf("create invoice title: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Title{}, fmt.Errorf("commit invoice title transaction: %w", err)
	}
	committed = true
	return title, nil
}

// UpdateTitle updates one non-deleted title owned by the user.
func (s *Store) UpdateTitle(ctx context.Context, userID, titleID int64, input TitleInput) (Title, error) {
	if s == nil || s.db == nil || userID <= 0 || titleID <= 0 {
		return Title{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Title{}, fmt.Errorf("begin invoice title update: %w", err)
	}
	committed := false
	defer rollbackUnlessCommitted(tx, &committed)

	if input.IsDefault {
		if _, err := tx.ExecContext(ctx, `
			UPDATE custom_invoice_titles
			SET is_default = FALSE, updated_at = NOW()
			WHERE user_id = $1 AND deleted_at IS NULL AND id <> $2
		`, userID, titleID); err != nil {
			return Title{}, fmt.Errorf("clear default invoice titles: %w", err)
		}
	}
	row := tx.QueryRowContext(ctx, `
		UPDATE custom_invoice_titles
		SET company_title = $3, tax_number = $4, receiver_email = $5,
		    is_default = $6, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING id, user_id, company_title, tax_number, receiver_email, is_default,
		          deleted_at, created_at, updated_at
	`, titleID, userID, input.CompanyTitle, input.TaxNumber, input.ReceiverEmail, input.IsDefault)
	title, err := scanTitle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Title{}, ErrTitleNotFound
		}
		return Title{}, fmt.Errorf("update invoice title: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Title{}, fmt.Errorf("commit invoice title update: %w", err)
	}
	committed = true
	return title, nil
}

// DeleteTitle soft-deletes one title. Historical applications keep their snapshot fields.
func (s *Store) DeleteTitle(ctx context.Context, userID, titleID int64) error {
	if s == nil || s.db == nil || userID <= 0 || titleID <= 0 {
		return ErrInvalidInput
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE custom_invoice_titles
		SET deleted_at = NOW(), is_default = FALSE, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, titleID, userID)
	if err != nil {
		return fmt.Errorf("delete invoice title: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrTitleNotFound
	}
	return nil
}

// SetDefaultTitle marks one title as default and clears all other defaults for the user.
func (s *Store) SetDefaultTitle(ctx context.Context, userID, titleID int64) (Title, error) {
	if s == nil || s.db == nil || userID <= 0 || titleID <= 0 {
		return Title{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Title{}, fmt.Errorf("begin default invoice title transaction: %w", err)
	}
	committed := false
	defer rollbackUnlessCommitted(tx, &committed)

	var exists int
	if err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM custom_invoice_titles
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, titleID, userID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Title{}, ErrTitleNotFound
		}
		return Title{}, fmt.Errorf("check default invoice title: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE custom_invoice_titles
		SET is_default = FALSE, updated_at = NOW()
		WHERE user_id = $1 AND deleted_at IS NULL
	`, userID); err != nil {
		return Title{}, fmt.Errorf("clear default invoice titles: %w", err)
	}
	row := tx.QueryRowContext(ctx, `
		UPDATE custom_invoice_titles
		SET is_default = TRUE, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING id, user_id, company_title, tax_number, receiver_email, is_default,
		          deleted_at, created_at, updated_at
	`, titleID, userID)
	title, err := scanTitle(row)
	if err != nil {
		return Title{}, fmt.Errorf("set default invoice title: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Title{}, fmt.Errorf("commit default invoice title transaction: %w", err)
	}
	committed = true
	return title, nil
}

// ListEligibleOrders returns current user's paid balance recharge orders not occupied by active applications.
func (s *Store) ListEligibleOrders(ctx context.Context, userID int64) ([]EligibleOrder, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT po.id, po.out_trade_no, po.amount::text, po.pay_amount::text,
		       COALESCE(po.provider_snapshot->>'currency', $2) AS currency,
		       po.payment_type, po.status, po.paid_at, po.completed_at, po.created_at
		FROM payment_orders po
		WHERE po.user_id = $1
		  AND po.order_type = $3
		  AND po.status = ANY($4)
		  AND NOT EXISTS (
			  SELECT 1
			  FROM custom_invoice_application_orders iao
			  JOIN custom_invoice_applications ia ON ia.id = iao.application_id
			  WHERE iao.order_id = po.id AND ia.status = ANY($5)
		  )
		ORDER BY po.paid_at DESC NULLS LAST, po.created_at DESC, po.id DESC
	`, userID, defaultCurrency, payment.OrderTypeBalance, pq.Array(invoiceableRechargeStatuses()), pq.Array(occupyingStatuses()))
	if err != nil {
		return nil, fmt.Errorf("list eligible invoice orders: %w", err)
	}
	defer rows.Close()
	return scanEligibleOrders(rows)
}

// CreateApplication creates one invoice application and occupies selected orders atomically.
func (s *Store) CreateApplication(ctx context.Context, input CreateApplicationInput) (Application, error) {
	if s == nil || s.db == nil || input.UserID <= 0 || input.TitleID <= 0 || len(input.OrderIDs) == 0 {
		return Application{}, ErrInvalidInput
	}
	var lastErr error
	for attempt := 0; attempt < createApplicationNoMaxAttempts; attempt++ {
		applicationNo, err := generateApplicationNo(s.now())
		if err != nil {
			return Application{}, err
		}
		app, err := s.createApplicationWithNumber(ctx, input, applicationNo)
		if err == nil {
			return app, nil
		}
		if isApplicationNoConflict(err) {
			lastErr = err
			continue
		}
		return Application{}, err
	}
	return Application{}, fmt.Errorf("generate unique invoice application number: %w", lastErr)
}

func (s *Store) createApplicationWithNumber(ctx context.Context, input CreateApplicationInput, applicationNo string) (Application, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Application{}, fmt.Errorf("begin invoice application transaction: %w", err)
	}
	committed := false
	defer rollbackUnlessCommitted(tx, &committed)

	title, err := s.getTitleForUpdate(ctx, tx, input.UserID, input.TitleID)
	if err != nil {
		return Application{}, err
	}
	orders, err := s.lockInvoiceableOrders(ctx, tx, input.UserID, input.OrderIDs)
	if err != nil {
		return Application{}, err
	}
	if len(orders) != len(input.OrderIDs) {
		return Application{}, ErrOrderNotEligible
	}
	if err := s.ensureOrdersNotOccupied(ctx, tx, input.OrderIDs); err != nil {
		return Application{}, err
	}
	total, currency, err := sumApplicationOrders(orders)
	if err != nil {
		return Application{}, err
	}

	row := tx.QueryRowContext(ctx, `
		INSERT INTO custom_invoice_applications (
			application_no, user_id, status, invoice_type, title_id, company_title, tax_number,
			receiver_email, total_amount, currency, order_count, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::decimal, $10, $11, NOW(), NOW())
		RETURNING id, application_no, user_id, status, invoice_type, title_id, company_title, tax_number,
		          receiver_email, total_amount::text, currency, order_count, invoice_number,
		          admin_remark, reject_reason, file_object_key, file_original_name, file_size,
		          issued_by, issued_at, rejected_by, rejected_at, created_at, updated_at
	`, applicationNo, input.UserID, StatusPending, InvoiceTypeEnterpriseVATNormal, title.ID, title.CompanyTitle, title.TaxNumber,
		title.ReceiverEmail, total, currency, len(orders))
	app, err := scanApplication(row)
	if err != nil {
		return Application{}, fmt.Errorf("create invoice application: %w", err)
	}
	for _, order := range orders {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO custom_invoice_application_orders (
				application_id, order_id, user_id, amount, currency, created_at
			) VALUES ($1, $2, $3, $4::decimal, $5, NOW())
			ON CONFLICT (application_id, order_id) DO NOTHING
		`, app.ID, order.ID, input.UserID, order.PayAmount, order.Currency); err != nil {
			return Application{}, fmt.Errorf("bind invoice order: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Application{}, fmt.Errorf("commit invoice application transaction: %w", err)
	}
	committed = true
	return s.GetApplication(ctx, app.ID, input.UserID)
}

// ListUserApplications lists applications belonging to one user.
func (s *Store) ListUserApplications(ctx context.Context, filter ListApplicationsFilter) ([]Application, int, error) {
	if s == nil || s.db == nil || filter.UserID <= 0 {
		return nil, 0, ErrInvalidInput
	}
	return s.listApplications(ctx, appListQuery{UserID: filter.UserID, Status: filter.Status, Page: filter.Page, PageSize: filter.PageSize})
}

// ListAdminApplications lists applications for admins.
func (s *Store) ListAdminApplications(ctx context.Context, filter AdminListApplicationsFilter) ([]Application, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, ErrInvalidInput
	}
	return s.listApplications(ctx, appListQuery{UserID: filter.UserID, Status: filter.Status, Page: filter.Page, PageSize: filter.PageSize})
}

// GetApplication loads one application. userID limits access when positive.
func (s *Store) GetApplication(ctx context.Context, id int64, userID int64) (Application, error) {
	if s == nil || s.db == nil || id <= 0 {
		return Application{}, ErrInvalidInput
	}
	where := "id = $1"
	args := []any{id}
	if userID > 0 {
		where += " AND user_id = $2"
		args = append(args, userID)
	}
	row := s.db.QueryRowContext(ctx, applicationSelectSQL()+" WHERE "+where, args...)
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Application{}, ErrApplicationNotFound
		}
		return Application{}, fmt.Errorf("get invoice application: %w", err)
	}
	orders, err := s.listApplicationOrders(ctx, app.ID)
	if err != nil {
		return Application{}, err
	}
	app.Orders = orders
	return app, nil
}

// IssueApplication marks a pending application as issued.
func (s *Store) IssueApplication(ctx context.Context, id int64, input IssueInput) (Application, error) {
	if s == nil || s.db == nil || id <= 0 || input.AdminID <= 0 {
		return Application{}, ErrInvalidInput
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE custom_invoice_applications
		SET status = $2, invoice_number = $3, admin_remark = $4, file_object_key = $5,
		    file_original_name = $6, file_size = $7, issued_by = $8, issued_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = $9
		RETURNING id, application_no, user_id, status, invoice_type, title_id, company_title, tax_number,
		          receiver_email, total_amount::text, currency, order_count, invoice_number,
		          admin_remark, reject_reason, file_object_key, file_original_name, file_size,
		          issued_by, issued_at, rejected_by, rejected_at, created_at, updated_at
	`, id, StatusIssued, input.InvoiceNumber, input.AdminRemark, input.FileObjectKey,
		input.FileOriginalName, input.FileSize, input.AdminID, StatusPending)
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Application{}, ErrInvalidStatus
		}
		return Application{}, fmt.Errorf("issue invoice application: %w", err)
	}
	orders, err := s.listApplicationOrders(ctx, app.ID)
	if err != nil {
		return Application{}, err
	}
	app.Orders = orders
	return app, nil
}

// RevertIssuedApplication restores a just-issued application when completion notification fails.
func (s *Store) RevertIssuedApplication(ctx context.Context, id int64, input IssueInput) error {
	if s == nil || s.db == nil || id <= 0 {
		return ErrInvalidInput
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE custom_invoice_applications
		SET status = $2, invoice_number = '', admin_remark = '', file_object_key = '',
		    file_original_name = '', file_size = 0, issued_by = NULL, issued_at = NULL, updated_at = NOW()
		WHERE id = $1 AND status = $3 AND file_object_key = $4 AND invoice_number = $5
	`, id, StatusPending, StatusIssued, input.FileObjectKey, input.InvoiceNumber)
	if err != nil {
		return fmt.Errorf("revert issued invoice application: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check reverted invoice application: %w", err)
	}
	if affected != 1 {
		return ErrInvalidStatus
	}
	return nil
}

// RejectApplication marks a pending application as rejected.
func (s *Store) RejectApplication(ctx context.Context, id int64, input RejectInput) (Application, error) {
	if s == nil || s.db == nil || id <= 0 || input.AdminID <= 0 {
		return Application{}, ErrInvalidInput
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE custom_invoice_applications
		SET status = $2, reject_reason = $3, rejected_by = $4, rejected_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = $5
		RETURNING id, application_no, user_id, status, invoice_type, title_id, company_title, tax_number,
		          receiver_email, total_amount::text, currency, order_count, invoice_number,
		          admin_remark, reject_reason, file_object_key, file_original_name, file_size,
		          issued_by, issued_at, rejected_by, rejected_at, created_at, updated_at
	`, id, StatusRejected, input.Reason, input.AdminID, StatusPending)
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Application{}, ErrInvalidStatus
		}
		return Application{}, fmt.Errorf("reject invoice application: %w", err)
	}
	orders, err := s.listApplicationOrders(ctx, app.ID)
	if err != nil {
		return Application{}, err
	}
	app.Orders = orders
	return app, nil
}
