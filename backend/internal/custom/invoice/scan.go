package invoice

import (
	"database/sql"
	"fmt"

	"github.com/shopspring/decimal"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTitle(row rowScanner) (Title, error) {
	var title Title
	if err := row.Scan(
		&title.ID,
		&title.UserID,
		&title.CompanyTitle,
		&title.TaxNumber,
		&title.ReceiverEmail,
		&title.IsDefault,
		&title.DeletedAt,
		&title.CreatedAt,
		&title.UpdatedAt,
	); err != nil {
		return Title{}, err
	}
	return title, nil
}

func scanTitles(rows *sql.Rows) ([]Title, error) {
	titles := []Title{}
	for rows.Next() {
		title, err := scanTitle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan invoice title: %w", err)
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice titles: %w", err)
	}
	return titles, nil
}

func scanEligibleOrders(rows *sql.Rows) ([]EligibleOrder, error) {
	orders := []EligibleOrder{}
	for rows.Next() {
		var order EligibleOrder
		if err := rows.Scan(
			&order.ID,
			&order.OutTradeNo,
			&order.Amount,
			&order.PayAmount,
			&order.Currency,
			&order.PaymentType,
			&order.Status,
			&order.PaidAt,
			&order.CompletedAt,
			&order.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan eligible invoice order: %w", err)
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate eligible invoice orders: %w", err)
	}
	return orders, nil
}

func scanApplication(row rowScanner) (Application, error) {
	var app Application
	if err := row.Scan(
		&app.ID,
		&app.ApplicationNo,
		&app.UserID,
		&app.Status,
		&app.InvoiceType,
		&app.TitleID,
		&app.CompanyTitle,
		&app.TaxNumber,
		&app.ReceiverEmail,
		&app.TotalAmount,
		&app.Currency,
		&app.OrderCount,
		&app.InvoiceNumber,
		&app.AdminRemark,
		&app.RejectReason,
		&app.FileObjectKey,
		&app.FileOriginalName,
		&app.FileSize,
		&app.IssuedBy,
		&app.IssuedAt,
		&app.RejectedBy,
		&app.RejectedAt,
		&app.CreatedAt,
		&app.UpdatedAt,
	); err != nil {
		return Application{}, err
	}
	return app, nil
}

func scanApplications(rows *sql.Rows) ([]Application, error) {
	apps := []Application{}
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan invoice application: %w", err)
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice applications: %w", err)
	}
	return apps, nil
}

func scanApplicationOrders(rows *sql.Rows) ([]ApplicationOrder, error) {
	orders := []ApplicationOrder{}
	for rows.Next() {
		var order ApplicationOrder
		if err := rows.Scan(
			&order.ApplicationID,
			&order.OrderID,
			&order.UserID,
			&order.Amount,
			&order.Currency,
			&order.OutTradeNo,
			&order.PaymentType,
			&order.Status,
			&order.PaidAt,
			&order.CompletedAt,
			&order.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan invoice application order: %w", err)
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice application orders: %w", err)
	}
	return orders, nil
}

func sumApplicationOrders(orders []EligibleOrder) (string, string, error) {
	if len(orders) == 0 {
		return "", "", ErrInvalidInput
	}
	currency := orders[0].Currency
	total := decimal.Zero
	for _, order := range orders {
		if order.Currency != currency {
			return "", "", ErrOrderNotEligible
		}
		amount, err := decimal.NewFromString(order.PayAmount)
		if err != nil || amount.LessThanOrEqual(decimal.Zero) {
			return "", "", ErrOrderNotEligible
		}
		total = total.Add(amount)
	}
	return total.StringFixed(8), currency, nil
}
