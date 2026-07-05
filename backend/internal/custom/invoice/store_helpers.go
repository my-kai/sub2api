package invoice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/lib/pq"
)

type appListQuery struct {
	UserID   int64
	Status   string
	Page     int
	PageSize int
}

func (s *Store) listApplications(ctx context.Context, filter appListQuery) ([]Application, int, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	clauses := []string{"1=1"}
	args := []any{}
	if filter.UserID > 0 {
		args = append(args, filter.UserID)
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if strings.TrimSpace(filter.Status) != "" {
		args = append(args, strings.TrimSpace(filter.Status))
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM custom_invoice_applications WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count invoice applications: %w", err)
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, applicationSelectSQL()+`
		WHERE `+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list invoice applications: %w", err)
	}
	defer rows.Close()
	apps, err := scanApplications(rows)
	if err != nil {
		return nil, 0, err
	}
	return apps, total, nil
}

func (s *Store) getTitleForUpdate(ctx context.Context, tx *sql.Tx, userID, titleID int64) (Title, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, user_id, company_title, tax_number, receiver_email, is_default,
		       deleted_at, created_at, updated_at
		FROM custom_invoice_titles
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, titleID, userID)
	title, err := scanTitle(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Title{}, ErrTitleNotFound
		}
		return Title{}, fmt.Errorf("get invoice title: %w", err)
	}
	return title, nil
}

func (s *Store) lockInvoiceableOrders(ctx context.Context, tx *sql.Tx, userID int64, orderIDs []int64) ([]EligibleOrder, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, out_trade_no, amount::text, pay_amount::text,
		       COALESCE(provider_snapshot->>'currency', $3) AS currency,
		       payment_type, status, paid_at, completed_at, created_at
		FROM payment_orders
		WHERE user_id = $1
		  AND id = ANY($2)
		  AND order_type = $4
		  AND status = ANY($5)
		ORDER BY id
		FOR UPDATE
	`, userID, pq.Array(orderIDs), defaultCurrency, payment.OrderTypeBalance, pq.Array(invoiceableRechargeStatuses()))
	if err != nil {
		return nil, fmt.Errorf("lock invoice orders: %w", err)
	}
	defer rows.Close()
	return scanEligibleOrders(rows)
}

func (s *Store) ensureOrdersNotOccupied(ctx context.Context, tx *sql.Tx, orderIDs []int64) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT iao.order_id
		FROM custom_invoice_application_orders iao
		JOIN custom_invoice_applications ia ON ia.id = iao.application_id
		WHERE iao.order_id = ANY($1) AND ia.status = ANY($2)
		LIMIT 1
	`, pq.Array(orderIDs), pq.Array(occupyingStatuses()))
	if err != nil {
		return fmt.Errorf("check invoice order occupation: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return ErrOrderOccupied
	}
	return rows.Err()
}

func (s *Store) listApplicationOrders(ctx context.Context, applicationID int64) ([]ApplicationOrder, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT iao.application_id, iao.order_id, iao.user_id, iao.amount::text, iao.currency,
		       po.out_trade_no, po.payment_type, po.status, po.paid_at, po.completed_at,
		       iao.created_at
		FROM custom_invoice_application_orders iao
		LEFT JOIN payment_orders po ON po.id = iao.order_id
		WHERE iao.application_id = $1
		ORDER BY iao.order_id
	`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list invoice application orders: %w", err)
	}
	defer rows.Close()
	return scanApplicationOrders(rows)
}

func invoiceableRechargeStatuses() []string {
	// Only COMPLETED means the balance recharge has finished crediting the user.
	// PAID/RECHARGING are intermediate states and must not become invoiceable.
	return []string{payment.OrderStatusCompleted}
}

func occupyingStatuses() []string {
	return []string{StatusPending, StatusIssued}
}

func applicationSelectSQL() string {
	return `SELECT id, application_no, user_id, status, invoice_type, title_id, company_title, tax_number,
			          receiver_email, total_amount::text, currency, order_count, invoice_number,
			          admin_remark, reject_reason, file_object_key, file_original_name, file_size,
			          issued_by, issued_at, rejected_by, rejected_at, created_at, updated_at
		   FROM custom_invoice_applications`
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func rollbackUnlessCommitted(tx *sql.Tx, committed *bool) {
	if tx != nil && (committed == nil || !*committed) {
		_ = tx.Rollback()
	}
}

func isApplicationNoConflict(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return string(pqErr.Code) == "23505" && pqErr.Constraint == "idx_custom_invoice_applications_application_no"
}
