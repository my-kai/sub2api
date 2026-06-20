/**
 * 生成红包雨弱设备指纹摘要。
 *
 * 该摘要只用于活动风控辅助判断，不作为实名身份，也不采集高敏指纹原文。
 */
export async function buildRedPacketRainFingerprint(): Promise<string> {
  const parts = [
    navigator.userAgent || '',
    navigator.language || '',
    Intl.DateTimeFormat().resolvedOptions().timeZone || '',
    `${window.screen.width}x${window.screen.height}`,
    String(window.devicePixelRatio || 1),
    typeof crypto !== 'undefined' && Boolean(crypto.subtle) ? 'crypto:1' : 'crypto:0',
  ]
  const data = new TextEncoder().encode(parts.join('|'))
  const digest = await crypto.subtle.digest('SHA-256', data)
  return Array.from(new Uint8Array(digest))
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}
