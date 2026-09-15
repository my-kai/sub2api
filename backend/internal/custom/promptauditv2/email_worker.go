package promptauditv2

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"
)

const (
	// emailPollInterval balances prompt delivery with idle database load.
	emailPollInterval = 2 * time.Second
	// emailLeaseDuration lets another instance recover work after a crashed sender.
	emailLeaseDuration = 45 * time.Second
	// emailMaxAttempts bounds repeated SMTP failures for each hit notification.
	emailMaxAttempts = 5
)

var emailRetryDelays = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	6 * time.Hour,
}

func (s *Service) runEmailWorker(ctx context.Context) {
	ticker := time.NewTicker(emailPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, err := s.store.ClaimEmailJob(ctx, emailLeaseDuration)
			if err != nil {
				s.setLastError(ErrorCodePersistence)
				continue
			}
			if job == nil {
				continue
			}
			s.deliverEmailJob(ctx, job)
		}
	}
}

func (s *Service) deliverEmailJob(ctx context.Context, job *EmailJob) {
	errCode := "prompt_audit_v2_email_delivery_failed"
	var sendErr error
	if strings.TrimSpace(job.RecipientEmail) == "" {
		sendErr = fmt.Errorf("recipient email is empty")
	} else {
		sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		sendErr = s.emailService.SendEmail(sendCtx, job.RecipientEmail, "账号使用风险提醒", riskEmailBody(job.Event))
		cancel()
	}
	if sendErr == nil {
		if err := s.store.CompleteEmailJob(ctx, job.ID); err != nil {
			s.setLastError(ErrorCodePersistence)
		}
		return
	}
	delayIndex := job.Attempts - 1
	if delayIndex < 0 {
		delayIndex = 0
	}
	if delayIndex >= len(emailRetryDelays) {
		delayIndex = len(emailRetryDelays) - 1
	}
	if err := s.store.FailEmailJob(ctx, job.ID, job.Attempts, emailMaxAttempts, time.Now().UTC().Add(emailRetryDelays[delayIndex]), errCode); err != nil {
		s.setLastError(ErrorCodePersistence)
	}
}

func riskEmailBody(event Event) string {
	action := "本次请求已记录，尚未达到限制次数"
	if event.ThresholdReached {
		if event.FinalAction == ActionBan {
			action = "账号已被永久禁用"
		} else {
			action = fmt.Sprintf("账号已限制使用 %d 分钟", valueOrZero(event.RuleRestrictionMinutes))
		}
	}
	return fmt.Sprintf(`<!doctype html><html><body>
<h2>账号使用风险提醒</h2>
<p>系统检测到一次符合安全规则的请求。</p>
<ul>
<li>触发时间：%s</li>
<li>风险原因：%s</li>
<li>置信度：%.4f</li>
<li>当前次数：%d / %d</li>
<li>处理结果：%s</li>
</ul>
</body></html>`,
		html.EscapeString(event.CreatedAt.Format(time.RFC3339)),
		html.EscapeString(event.Reason), event.Confidence,
		event.WindowHitCount, event.RuleTriggerCount, html.EscapeString(action),
	)
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
