package aigatewayadmintransfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

const (
	// transferMoneyScale 固定来源与目标普通余额之间允许的 USD 八位小数精度。
	transferMoneyScale int32 = 8
	// maxTransferIDLength 限制稳定转入 ID，避免无界索引值进入来源账本。
	maxTransferIDLength = 128
)

// transferColumns 固定来源扣款记录的扫描列顺序，避免状态查询遗漏精确金额。
const transferColumns = `
	transfer_id,
	user_id,
	amount_usd::text,
	status,
	created_at,
	updated_at
`

// Store 负责来源普通余额条件扣减与稳定转入记录的 SQL 持久化。
type Store struct {
	db *sql.DB
}

// NewStore 创建来源扣款存储；调用方必须先完成本领域 custom migration。
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("ai-gateway admin transfer sql db is required")
	}
	return &Store{db: db}, nil
}

// LoadRegularBalance 返回来源用户的普通余额文本，不读取赠送、冻结或其他派生余额。
func (s *Store) LoadRegularBalance(ctx context.Context, userID int64) (string, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return "", ErrInvalidInput
	}
	var balance string
	err := s.db.QueryRowContext(ctx, `
		SELECT balance::text
		FROM users
		WHERE id = $1
		  AND status = 'active'
		  AND deleted_at IS NULL
	`, userID).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("load ai-gateway source regular balance: %w", err)
	}
	availableBalance, ok := normalizeAvailableBalance(balance)
	if !ok {
		return "", fmt.Errorf("source regular balance has invalid numeric format: %w", ErrServiceUnavailable)
	}
	return availableBalance, nil
}

// CreateAndDebit 在同一事务内条件扣减来源普通余额并保存稳定转入记录。
func (s *Store) CreateAndDebit(ctx context.Context, candidate Transfer) (Transfer, error) {
	if s == nil || s.db == nil || candidate.UserID <= 0 || !validTransferID(candidate.TransferID) || normalizeAmount(candidate.AmountUSD) == "" {
		return Transfer{}, ErrInvalidInput
	}
	candidate.AmountUSD = normalizeAmount(candidate.AmountUSD)
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Transfer{}, fmt.Errorf("begin ai-gateway admin transfer debit transaction: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()

	if existing, found, err := findTransfer(ctx, transaction, candidate.TransferID); err != nil {
		return Transfer{}, err
	} else if found {
		return matchingTransfer(existing, candidate)
	}

	result, err := transaction.ExecContext(ctx, `
		UPDATE users
		SET balance = balance - $1::numeric,
		    updated_at = NOW()
		WHERE id = $2
		  AND status = 'active'
		  AND deleted_at IS NULL
		  AND balance >= $1::numeric
	`, candidate.AmountUSD, candidate.UserID)
	if err != nil {
		return Transfer{}, fmt.Errorf("deduct ai-gateway admin transfer balance: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Transfer{}, fmt.Errorf("read ai-gateway admin transfer debit result: %w", err)
	}
	if rowsAffected != 1 {
		return Transfer{}, ErrInsufficientBalance
	}

	stored, err := insertTransfer(ctx, transaction, candidate)
	if err != nil {
		if isUniqueViolation(err) {
			if rollbackErr := transaction.Rollback(); rollbackErr != nil {
				return Transfer{}, fmt.Errorf("rollback ai-gateway admin transfer unique race: %w", rollbackErr)
			}
			return s.loadConcurrentTransfer(ctx, candidate)
		}
		return Transfer{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Transfer{}, fmt.Errorf("commit ai-gateway admin transfer debit transaction: %w", err)
	}
	return stored, nil
}

// FindByTransferID 查询来源账本中的稳定转入记录，供目标恢复流程安全判断是否已扣款。
func (s *Store) FindByTransferID(ctx context.Context, transferID string) (Transfer, bool, error) {
	if s == nil || s.db == nil || !validTransferID(transferID) {
		return Transfer{}, false, ErrInvalidInput
	}
	return findTransfer(ctx, s.db, transferID)
}

// loadConcurrentTransfer 在唯一索引竞争回滚后读取获胜记录，确保同一请求不重复扣款。
func (s *Store) loadConcurrentTransfer(ctx context.Context, candidate Transfer) (Transfer, error) {
	existing, found, err := s.FindByTransferID(ctx, candidate.TransferID)
	if err != nil {
		return Transfer{}, err
	}
	if !found {
		return Transfer{}, ErrTransferConflict
	}
	return matchingTransfer(existing, candidate)
}

// matchingTransfer 验证幂等重试仍对应同一来源用户与金额，避免稳定 ID 被复用篡改。
func matchingTransfer(existing, candidate Transfer) (Transfer, error) {
	if existing.UserID != candidate.UserID || existing.AmountUSD != candidate.AmountUSD || existing.Status != transferStatusDebited {
		return Transfer{}, ErrTransferConflict
	}
	return existing, nil
}

// insertTransfer 保存已完成扣款的来源账本记录，并返回数据库规范化后的字段。
func insertTransfer(ctx context.Context, transaction *sql.Tx, candidate Transfer) (Transfer, error) {
	row := transaction.QueryRowContext(ctx, `
		INSERT INTO custom_ai_gateway_admin_transfers (
			transfer_id,
			user_id,
			amount_usd,
			status
		) VALUES ($1, $2, $3::numeric, $4)
		RETURNING `+transferColumns,
		candidate.TransferID,
		candidate.UserID,
		candidate.AmountUSD,
		transferStatusDebited,
	)
	transfer, err := scanTransfer(row)
	if err != nil {
		return Transfer{}, fmt.Errorf("insert ai-gateway admin transfer: %w", err)
	}
	return transfer, nil
}

// rowQuerier 统一数据库连接和事务的单行查询能力，保证状态查询与事务查询共用扫描逻辑。
type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// findTransfer 按稳定转入 ID 查询来源记录，并显式区分不存在与存储失败。
func findTransfer(ctx context.Context, queryer rowQuerier, transferID string) (Transfer, bool, error) {
	row := queryer.QueryRowContext(ctx, `
		SELECT `+transferColumns+`
		FROM custom_ai_gateway_admin_transfers
		WHERE transfer_id = $1
	`, transferID)
	transfer, err := scanTransfer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Transfer{}, false, nil
	}
	if err != nil {
		return Transfer{}, false, fmt.Errorf("find ai-gateway admin transfer: %w", err)
	}
	return transfer, true, nil
}

// scanTransfer 将固定列顺序映射为来源转入记录，金额保持精确文本而非浮点数。
func scanTransfer(row *sql.Row) (Transfer, error) {
	var transfer Transfer
	if err := row.Scan(
		&transfer.TransferID,
		&transfer.UserID,
		&transfer.AmountUSD,
		&transfer.Status,
		&transfer.CreatedAt,
		&transfer.UpdatedAt,
	); err != nil {
		return Transfer{}, err
	}
	transfer.AmountUSD = normalizeAmount(transfer.AmountUSD)
	return transfer, nil
}

// validTransferID 拒绝空白、换行和超长 ID，避免来源账本索引被异常输入放大。
func validTransferID(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && len(value) <= maxTransferIDLength && !strings.ContainsAny(value, "\r\n")
}

// normalizeAmount 将来源与目标共同支持的 USD 金额规范为固定八位小数文本。
func normalizeAmount(value string) string {
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || !parsed.IsPositive() || parsed.Exponent() < -transferMoneyScale {
		return ""
	}
	return parsed.StringFixed(transferMoneyScale)
}

// normalizeAvailableBalance 将来源普通余额转换为可转余额；零和负余额都不能触发扣款，统一返回零。
func normalizeAvailableBalance(value string) (string, bool) {
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil || parsed.Exponent() < -transferMoneyScale {
		return "", false
	}
	if !parsed.IsPositive() {
		return decimal.Zero.StringFixed(transferMoneyScale), true
	}
	return parsed.StringFixed(transferMoneyScale), true
}

// isUniqueViolation 仅识别 PostgreSQL 唯一约束竞争，其他数据库错误必须显式向上返回。
func isUniqueViolation(err error) bool {
	var postgresError *pq.Error
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}

// nowUTC 保证记录状态的时间比较在统一时区下完成，避免来源和目标时区差异影响恢复。
func nowUTC() time.Time {
	return time.Now().UTC()
}
