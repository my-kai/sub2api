import type {
  RedPacketRainEncryptedClaimPayload,
  RedPacketRainWSChallengeMessage,
  RedPacketRainWSClaimMessage,
} from '../types'

const textEncoder = new TextEncoder()

/**
 * 生成 URL 安全随机值。
 *
 * 用于 client nonce 和 WebSocket claim nonce，避免跨轮或跨会话复用。
 */
export function createRedPacketRainNonce(byteLength = 24): string {
  const bytes = new Uint8Array(byteLength)
  crypto.getRandomValues(bytes)
  return toBase64URL(bytes)
}

/**
 * 生成幂等 key。
 *
 * 该值只用于服务端重复提交保护，不参与奖励金额计算。
 */
export function createRedPacketRainIdempotencyKey(activityID: number, roundID: number): string {
  return `activity-${activityID}-round-${roundID}-${Date.now()}-${createRedPacketRainNonce(12)}`
}

/**
 * 派生当前 WebSocket 会话密钥。
 *
 * 密钥材料全部来自本次短期 ticket 和服务端 challenge；页面卸载后应丢弃。
 */
export async function deriveRedPacketRainSessionKey(
  ticket: string,
  challenge: RedPacketRainWSChallengeMessage,
): Promise<CryptoKey> {
  const ticketHash = await sha256Hex(ticket)
  const material = [ticketHash, challenge.session_id, challenge.server_nonce, challenge.challenge].join('\0')
  const digest = await crypto.subtle.digest('SHA-256', textEncoder.encode(material))
  return crypto.subtle.importKey('raw', digest, { name: 'AES-GCM' }, false, ['encrypt'])
}

/**
 * 加密领取载荷并生成签名。
 */
export async function buildEncryptedRedPacketRainClaim(input: {
  activityID: number
  roundID: number
  ticket: string
  challenge: RedPacketRainWSChallengeMessage
  idempotencyKey: string
  payload: RedPacketRainEncryptedClaimPayload
}): Promise<RedPacketRainWSClaimMessage> {
  const key = await deriveRedPacketRainSessionKey(input.ticket, input.challenge)
  const nonce = createRedPacketRainNonce(12)
  const encodedNonce = fromBase64URL(nonce)
  const aad = textEncoder.encode(`${input.challenge.session_id}:${input.roundID}:${input.idempotencyKey}`)
  const ciphertext = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: encodedNonce, additionalData: aad },
    key,
    textEncoder.encode(JSON.stringify(input.payload)),
  )
  const ciphertextText = toBase64URL(new Uint8Array(ciphertext))
  const signature = await signRedPacketRainClaim(input.ticket, input.challenge, {
    roundID: input.roundID,
    idempotencyKey: input.idempotencyKey,
    nonce,
    ciphertext: ciphertextText,
  })

  return {
    type: 'claim',
    session_id: input.challenge.session_id,
    round_id: input.roundID,
    idempotency_key: input.idempotencyKey,
    nonce,
    ciphertext: ciphertextText,
    signature,
  }
}

async function signRedPacketRainClaim(
  ticket: string,
  challenge: RedPacketRainWSChallengeMessage,
  input: { roundID: number, idempotencyKey: string, nonce: string, ciphertext: string },
): Promise<string> {
  const ticketHash = await sha256Hex(ticket)
  const keyBytes = await crypto.subtle.digest(
    'SHA-256',
    textEncoder.encode([ticketHash, challenge.session_id, challenge.server_nonce, challenge.challenge].join('\0')),
  )
  const key = await crypto.subtle.importKey('raw', keyBytes, { name: 'HMAC', hash: 'SHA-256' }, false, ['sign'])
  const payload = [
    challenge.session_id,
    String(input.roundID),
    input.idempotencyKey,
    input.nonce,
    input.ciphertext,
  ].join('\0') + '\0'
  const signature = await crypto.subtle.sign('HMAC', key, textEncoder.encode(payload))
  return toBase64URL(new Uint8Array(signature))
}

async function sha256Hex(value: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', textEncoder.encode(value.trim()))
  return Array.from(new Uint8Array(digest))
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}

function toBase64URL(bytes: Uint8Array): string {
  let binary = ''
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte)
  })
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

function fromBase64URL(value: string): Uint8Array {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }
  return bytes
}
