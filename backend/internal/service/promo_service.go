package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	gifttypes "github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/types"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrPromoCodeNotFound         = infraerrors.NotFound("PROMO_CODE_NOT_FOUND", "promo code not found")
	ErrPromoCodeExpired          = infraerrors.BadRequest("PROMO_CODE_EXPIRED", "promo code has expired")
	ErrPromoCodeDisabled         = infraerrors.BadRequest("PROMO_CODE_DISABLED", "promo code is disabled")
	ErrPromoCodeMaxUsed          = infraerrors.BadRequest("PROMO_CODE_MAX_USED", "promo code has reached maximum uses")
	ErrPromoCodeAlreadyUsed      = infraerrors.Conflict("PROMO_CODE_ALREADY_USED", "you have already used this promo code")
	ErrPromoCodeInvalid          = infraerrors.BadRequest("PROMO_CODE_INVALID", "invalid promo code")
	ErrPromoCreditTypeRequired   = infraerrors.BadRequest("PROMO_CREDIT_TYPE_REQUIRED", "promo code credit type must be balance or gift")
	ErrPromoGiftValidityRequired = infraerrors.BadRequest("PROMO_GIFT_VALIDITY_REQUIRED", "gift promo code validity days must be greater than 0")
)

const promoMetaPrefix = "<!-- sub2api_custom_promo_meta:"
const promoMetaSuffix = " -->"

// PromoService 优惠码服务
type PromoService struct {
	promoRepo            PromoCodeRepository
	userRepo             UserRepository
	billingCacheService  *BillingCacheService
	entClient            *dbent.Client
	authCacheInvalidator APIKeyAuthCacheInvalidator
}

// NewPromoService 创建优惠码服务实例
func NewPromoService(
	promoRepo PromoCodeRepository,
	userRepo UserRepository,
	billingCacheService *BillingCacheService,
	entClient *dbent.Client,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
) *PromoService {
	return &PromoService{
		promoRepo:            promoRepo,
		userRepo:             userRepo,
		billingCacheService:  billingCacheService,
		entClient:            entClient,
		authCacheInvalidator: authCacheInvalidator,
	}
}

// ValidatePromoCode 验证优惠码（注册前调用）
// 返回 nil, nil 表示空码（不报错）
func (s *PromoService) ValidatePromoCode(ctx context.Context, code string) (*PromoCode, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil // 空码不报错，直接返回
	}

	promoCode, err := s.promoRepo.GetByCode(ctx, code)
	if err != nil {
		// 保留原始错误类型，不要统一映射为 NotFound
		return nil, err
	}

	if err := s.validatePromoCodeStatus(promoCode); err != nil {
		return nil, err
	}
	normalizePromoCodeCreditFields(promoCode)
	if err := validatePromoCreditFields(promoCode.CreditType, promoCode.GiftValidityDays); err != nil {
		return nil, err
	}

	return promoCode, nil
}

// validatePromoCodeStatus 验证优惠码状态
func (s *PromoService) validatePromoCodeStatus(promoCode *PromoCode) error {
	if !promoCode.CanUse() {
		if promoCode.IsExpired() {
			return ErrPromoCodeExpired
		}
		if promoCode.Status == PromoCodeStatusDisabled {
			return ErrPromoCodeDisabled
		}
		if promoCode.MaxUses > 0 && promoCode.UsedCount >= promoCode.MaxUses {
			return ErrPromoCodeMaxUsed
		}
		return ErrPromoCodeInvalid
	}
	return nil
}

// ApplyPromoCode 应用优惠码（注册成功后调用）
// 使用事务和行锁确保并发安全
func (s *PromoService) ApplyPromoCode(ctx context.Context, userID int64, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}

	// 开启事务
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)

	// 在事务中获取并锁定优惠码记录（FOR UPDATE）
	promoCode, err := s.promoRepo.GetByCodeForUpdate(txCtx, code)
	if err != nil {
		return err
	}

	// 在事务中验证优惠码状态
	if err := s.validatePromoCodeStatus(promoCode); err != nil {
		return err
	}
	normalizePromoCodeCreditFields(promoCode)

	// 在事务中检查用户是否已使用过此优惠码
	existing, err := s.promoRepo.GetUsageByPromoCodeAndUser(txCtx, promoCode.ID, userID)
	if err != nil {
		return fmt.Errorf("check existing usage: %w", err)
	}
	if existing != nil {
		return ErrPromoCodeAlreadyUsed
	}

	usedAt := time.Now().UTC()
	usage := &PromoCodeUsage{
		PromoCodeID: promoCode.ID,
		UserID:      userID,
		BonusAmount: promoCode.BonusAmount,
		UsedAt:      usedAt,
	}

	if promoCode.CreditType == creditTypeGift {
		if err := validatePromoGiftValidityDays(promoCode.GiftValidityDays); err != nil {
			return err
		}
		if err := s.promoRepo.CreateUsage(txCtx, usage); err != nil {
			return fmt.Errorf("create usage record: %w", err)
		}
		if _, err := createGiftCreditGrantSQL(ctx, tx.Client(), gifttypes.CreateGrantInput{
			UserID:     userID,
			SourceType: gifttypes.SourcePromoCode,
			SourceID:   fmt.Sprintf("promo:%d:usage:%d", promoCode.ID, usage.ID),
			Amount:     formatGiftAmount(promoCode.BonusAmount),
			ExpiresAt:  usedAt.AddDate(0, 0, promoCode.GiftValidityDays),
			Note:       "优惠码赠送余额",
			CreatedAt:  usedAt,
		}); err != nil {
			return fmt.Errorf("create promo gift credit grant: %w", err)
		}
	} else {
		// 普通余额优惠码保留原有 users.balance 入账语义，避免影响注册赠送旧入口。
		if err := s.userRepo.UpdateBalance(txCtx, userID, promoCode.BonusAmount); err != nil {
			return fmt.Errorf("update user balance: %w", err)
		}
		if err := s.promoRepo.CreateUsage(txCtx, usage); err != nil {
			return fmt.Errorf("create usage record: %w", err)
		}
	}

	// 增加使用次数
	if err := s.promoRepo.IncrementUsedCount(txCtx, promoCode.ID); err != nil {
		return fmt.Errorf("increment used count: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	s.invalidatePromoCaches(ctx, userID, promoCode.BonusAmount)

	// 失效余额缓存
	if s.billingCacheService != nil {
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
		}()
	}

	return nil
}

func (s *PromoService) invalidatePromoCaches(ctx context.Context, userID int64, bonusAmount float64) {
	if bonusAmount == 0 || s.authCacheInvalidator == nil {
		return
	}
	s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
}

// GenerateRandomCode 生成随机优惠码
func (s *PromoService) GenerateRandomCode() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return strings.ToUpper(hex.EncodeToString(bytes)), nil
}

// Create 创建优惠码
func (s *PromoService) Create(ctx context.Context, input *CreatePromoCodeInput) (*PromoCode, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		// 自动生成
		var err error
		code, err = s.GenerateRandomCode()
		if err != nil {
			return nil, err
		}
	}
	creditType, err := normalizeRequiredCreditType(input.CreditType)
	if err != nil {
		return nil, err
	}
	giftValidityDays := normalizeGiftValidityDays(input.GiftValidityDays)
	if err := validatePromoCreditFields(creditType, giftValidityDays); err != nil {
		return nil, err
	}
	notes, err := buildPromoNotesWithMeta(input.Notes, creditType, giftValidityDays)
	if err != nil {
		return nil, err
	}

	promoCode := &PromoCode{
		Code:             strings.ToUpper(code),
		BonusAmount:      input.BonusAmount,
		MaxUses:          input.MaxUses,
		UsedCount:        0,
		Status:           PromoCodeStatusActive,
		ExpiresAt:        input.ExpiresAt,
		Notes:            notes,
		CreditType:       creditType,
		GiftValidityDays: giftValidityDays,
	}

	if err := s.promoRepo.Create(ctx, promoCode); err != nil {
		return nil, fmt.Errorf("create promo code: %w", err)
	}
	normalizePromoCodeCreditFields(promoCode)

	return promoCode, nil
}

// GetByID 根据ID获取优惠码
func (s *PromoService) GetByID(ctx context.Context, id int64) (*PromoCode, error) {
	code, err := s.promoRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	normalizePromoCodeCreditFields(code)
	return code, nil
}

// Update 更新优惠码
func (s *PromoService) Update(ctx context.Context, id int64, input *UpdatePromoCodeInput) (*PromoCode, error) {
	promoCode, err := s.promoRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	normalizePromoCodeCreditFields(promoCode)

	if input.Code != nil {
		promoCode.Code = strings.ToUpper(strings.TrimSpace(*input.Code))
	}
	if input.BonusAmount != nil {
		promoCode.BonusAmount = *input.BonusAmount
	}
	if input.MaxUses != nil {
		promoCode.MaxUses = *input.MaxUses
	}
	if input.Status != nil {
		promoCode.Status = *input.Status
	}
	if input.ExpiresAt != nil {
		promoCode.ExpiresAt = input.ExpiresAt
	}
	if input.Notes != nil {
		promoCode.Notes = *input.Notes
	}
	creditType := promoCode.CreditType
	if input.CreditType != nil {
		creditType, err = normalizeRequiredCreditType(*input.CreditType)
		if err != nil {
			return nil, err
		}
	}
	giftValidityDays := promoCode.GiftValidityDays
	if input.GiftValidityDays != nil {
		giftValidityDays = *input.GiftValidityDays
	}
	promoCode.CreditType = creditType
	promoCode.GiftValidityDays = normalizeGiftValidityDays(giftValidityDays)
	if err := validatePromoCreditFields(promoCode.CreditType, promoCode.GiftValidityDays); err != nil {
		return nil, err
	}
	notes, err := buildPromoNotesWithMeta(promoCode.Notes, promoCode.CreditType, promoCode.GiftValidityDays)
	if err != nil {
		return nil, err
	}
	promoCode.Notes = notes

	if err := s.promoRepo.Update(ctx, promoCode); err != nil {
		return nil, fmt.Errorf("update promo code: %w", err)
	}
	normalizePromoCodeCreditFields(promoCode)

	return promoCode, nil
}

// Delete 删除优惠码
func (s *PromoService) Delete(ctx context.Context, id int64) error {
	if err := s.promoRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete promo code: %w", err)
	}
	return nil
}

// List 获取优惠码列表
func (s *PromoService) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]PromoCode, *pagination.PaginationResult, error) {
	codes, result, err := s.promoRepo.ListWithFilters(ctx, params, status, search)
	if err != nil {
		return nil, nil, err
	}
	for i := range codes {
		normalizePromoCodeCreditFields(&codes[i])
	}
	return codes, result, nil
}

// ListUsages 获取使用记录
func (s *PromoService) ListUsages(ctx context.Context, promoCodeID int64, params pagination.PaginationParams) ([]PromoCodeUsage, *pagination.PaginationResult, error) {
	return s.promoRepo.ListUsagesByPromoCode(ctx, promoCodeID, params)
}

type promoCodeCreditMeta struct {
	CreditType       string `json:"credit_type,omitempty"`
	GiftValidityDays int    `json:"gift_validity_days,omitempty"`
}

func normalizePromoCodeCreditFields(code *PromoCode) {
	if code == nil {
		return
	}
	note, meta := parsePromoNotesMeta(code.Notes)
	code.Notes = note
	code.CreditType = normalizeCreditType(firstPromoMetaNonEmpty(code.CreditType, meta.CreditType))
	code.GiftValidityDays = normalizeGiftValidityDays(firstPromoMetaPositive(code.GiftValidityDays, meta.GiftValidityDays))
	if code.CreditType == creditTypeBalance {
		code.GiftValidityDays = 0
	}
}

func buildPromoNotesWithMeta(note string, creditType string, giftValidityDays int) (string, error) {
	cleanNote, _ := parsePromoNotesMeta(note)
	normalizedCreditType := normalizeCreditType(creditType)
	normalizedGiftValidityDays := normalizeGiftValidityDays(giftValidityDays)
	if err := validatePromoCreditFields(normalizedCreditType, normalizedGiftValidityDays); err != nil {
		return "", err
	}
	meta := promoCodeCreditMeta{
		CreditType:       normalizedCreditType,
		GiftValidityDays: normalizedGiftValidityDays,
	}
	if meta.CreditType == creditTypeBalance {
		meta.GiftValidityDays = 0
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshal promo credit meta: %w", err)
	}
	if strings.TrimSpace(cleanNote) == "" {
		return promoMetaPrefix + string(payload) + promoMetaSuffix, nil
	}
	return cleanNote + "\n" + promoMetaPrefix + string(payload) + promoMetaSuffix, nil
}

func parsePromoNotesMeta(note string) (string, promoCodeCreditMeta) {
	raw := strings.TrimSpace(note)
	if raw == "" {
		return "", promoCodeCreditMeta{CreditType: creditTypeBalance}
	}
	start := strings.Index(raw, promoMetaPrefix)
	if start < 0 {
		return raw, promoCodeCreditMeta{CreditType: creditTypeBalance}
	}
	metaStart := start + len(promoMetaPrefix)
	metaEnd := strings.Index(raw[metaStart:], promoMetaSuffix)
	if metaEnd < 0 {
		return raw, promoCodeCreditMeta{CreditType: creditTypeBalance}
	}
	metaEnd += metaStart
	var meta promoCodeCreditMeta
	if err := json.Unmarshal([]byte(raw[metaStart:metaEnd]), &meta); err != nil {
		return raw, promoCodeCreditMeta{CreditType: creditTypeBalance}
	}
	clean := strings.TrimSpace(raw[:start] + raw[metaEnd+len(promoMetaSuffix):])
	return clean, meta
}

func firstPromoMetaNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstPromoMetaPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func validatePromoCreditFields(creditType string, giftValidityDays int) error {
	switch strings.TrimSpace(creditType) {
	case creditTypeBalance:
		return nil
	case creditTypeGift:
		return validatePromoGiftValidityDays(giftValidityDays)
	default:
		return ErrPromoCreditTypeRequired
	}
}

func validatePromoGiftValidityDays(days int) error {
	if days <= 0 {
		return ErrPromoGiftValidityRequired
	}
	return nil
}

func normalizeRequiredCreditType(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case creditTypeBalance:
		return creditTypeBalance, nil
	case creditTypeGift:
		return creditTypeGift, nil
	default:
		return "", ErrPromoCreditTypeRequired
	}
}
