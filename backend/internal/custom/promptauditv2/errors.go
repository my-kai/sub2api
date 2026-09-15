package promptauditv2

import (
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// ErrorCodeInvalidConfig identifies configuration payloads that cannot become a runtime snapshot.
	ErrorCodeInvalidConfig = "prompt_audit_v2_invalid_config"
	// ErrorCodeConfigConflict identifies optimistic-lock conflicts during a full configuration save.
	ErrorCodeConfigConflict = "prompt_audit_v2_config_conflict"
	// ErrorCodeQueueFull identifies requests rejected before model work because the bounded queue is full.
	ErrorCodeQueueFull = "prompt_audit_v2_queue_full"
	// ErrorCodeUnavailable identifies requests that cannot receive an authoritative audit decision.
	ErrorCodeUnavailable = "prompt_audit_v2_unavailable"
	// ErrorCodeInvalidResponse identifies an endpoint response that violates the strict JSON contract.
	ErrorCodeInvalidResponse = "prompt_audit_v2_invalid_response"
	// ErrorCodeRuleTriggered identifies the request that reaches a configured enforcement threshold.
	ErrorCodeRuleTriggered = "prompt_audit_v2_rule_triggered"
	// ErrorCodeUserBanned identifies a permanent restriction created by this module.
	ErrorCodeUserBanned = "prompt_audit_v2_user_banned"
	// ErrorCodeUserRestricted identifies an unexpired timed restriction created by this module.
	ErrorCodeUserRestricted = "prompt_audit_v2_user_restricted"
	// ErrorCodePersistence identifies a hit whose required transactional side effects could not be saved.
	ErrorCodePersistence = "prompt_audit_v2_persistence_failed"
	// ErrorCodeDecrypt identifies an event whose complete message cannot be decrypted for an administrator.
	ErrorCodeDecrypt = "prompt_audit_v2_event_decrypt_failed"
	// ErrorCodeNotFound identifies an event ID that is not present in the hit-only log.
	ErrorCodeNotFound = "prompt_audit_v2_event_not_found"
)

var (
	// ErrInvalidConfig is returned after field-specific validation has rejected a configuration.
	ErrInvalidConfig = infraerrors.BadRequest(ErrorCodeInvalidConfig, "提示词审计2配置无效")
	// ErrConfigConflict requires the administrator to reload before overwriting a newer configuration.
	ErrConfigConflict = infraerrors.Conflict(ErrorCodeConfigConflict, "配置已更新，请刷新后重试")
	// ErrEventNotFound prevents an absent event from being represented as an empty detail record.
	ErrEventNotFound = infraerrors.NotFound(ErrorCodeNotFound, "提示词审计2事件不存在")
	// ErrEventDecrypt keeps ciphertext out of the API when the configured encryption key cannot decrypt it.
	ErrEventDecrypt = infraerrors.New(http.StatusInternalServerError, ErrorCodeDecrypt, "完整消息解密失败")
	// ErrPromptTestUnavailable reports that every enabled draft endpoint failed without exposing provider details.
	ErrPromptTestUnavailable = infraerrors.ServiceUnavailable(ErrorCodeUnavailable, "所有模型服务测试失败，请检查模型服务配置")
)
