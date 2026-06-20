package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
	"github.com/stretchr/testify/require"
)

func TestValidateWSClaimPayloadRejectsFingerprintMismatch(t *testing.T) {
	svc := NewService(nil)
	err := svc.validateWSClaimPayload(types.RedPacketRainWSSession{
		DeviceFingerprint: "device-a",
		ClientNonce:       "client-nonce",
	}, WSClaimPayload{
		HitCount:          1,
		ClickTraceDigest:  "trace-digest",
		DeviceFingerprint: "device-b",
		ClientNonce:       "client-nonce",
	})

	require.ErrorIs(t, err, types.ErrRedPacketRainSecurityRejected)
}

func TestValidateWSClaimPayloadAllowsZeroHitWithoutTraceDigest(t *testing.T) {
	svc := NewService(nil)
	err := svc.validateWSClaimPayload(types.RedPacketRainWSSession{
		DeviceFingerprint: "device-a",
		ClientNonce:       "client-nonce",
	}, WSClaimPayload{
		HitCount:          0,
		DeviceFingerprint: "device-a",
		ClientNonce:       "client-nonce",
	})

	require.NoError(t, err)
}

func TestValidateWSClaimPayloadRejectsInvalidTimeOrder(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	svc := NewService(nil)
	err := svc.validateWSClaimPayload(types.RedPacketRainWSSession{
		DeviceFingerprint: "device-a",
		ClientNonce:       "client-nonce",
	}, WSClaimPayload{
		HitCount:          1,
		StartedAt:         now,
		EndedAt:           now.Add(-time.Second),
		ClickTraceDigest:  "trace-digest",
		DeviceFingerprint: "device-a",
		ClientNonce:       "client-nonce",
	})

	require.ErrorIs(t, err, types.ErrRedPacketRainSecurityRejected)
}
