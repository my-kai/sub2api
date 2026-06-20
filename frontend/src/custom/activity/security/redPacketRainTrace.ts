interface TracePoint {
  packetID: number
  x: number
  y: number
  at: number
}

/**
 * 红包雨点击轨迹摘要采集器。
 *
 * 页面只提交摘要，不上传完整轨迹，既能做基础行为校验，也避免审计数据过重。
 */
export class RedPacketRainTraceRecorder {
  private readonly points: TracePoint[] = []
  private startedAt = Date.now()

  recordHit(packetID: number, event?: MouseEvent): void {
    this.points.push({
      packetID,
      x: normalizeCoord(event?.clientX),
      y: normalizeCoord(event?.clientY),
      at: Date.now(),
    })
  }

  reset(): void {
    this.points.splice(0, this.points.length)
    this.startedAt = Date.now()
  }

  hitCount(): number {
    return this.points.length
  }

  startedAtISO(): string {
    return new Date(this.startedAt).toISOString()
  }

  endedAtISO(): string {
    return new Date().toISOString()
  }

  async digest(): Promise<string> {
    if (this.points.length === 0) {
      return ''
    }
    const normalized = this.points
      .map((point) => `${point.packetID}:${point.x}:${point.y}:${Math.max(0, point.at - this.startedAt)}`)
      .join('|')
    const data = new TextEncoder().encode(normalized)
    const digest = await crypto.subtle.digest('SHA-256', data)
    return Array.from(new Uint8Array(digest))
      .map((byte) => byte.toString(16).padStart(2, '0'))
      .join('')
  }
}

function normalizeCoord(value: number | undefined): number {
  if (!Number.isFinite(value)) {
    return 0
  }
  return Math.max(0, Math.round(Number(value)))
}
