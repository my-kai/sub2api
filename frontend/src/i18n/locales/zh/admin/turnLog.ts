export default {
  turnLog: {
    title: 'turn记录',
    description: '查看 OpenAI OAuth 上游异常响应记录',
    empty: '暂无 turn 记录',
    truncated: '响应体已截断',
    complete: '响应体完整',
    partial: '响应体未完整读取',
    filters: { accountId: '账号 ID', status: '状态码' },
    columns: { time: '时间', account: '账号', status: '状态码', payload: '响应内容', headers: '响应 headers', body: '响应 body' },
    config: { retentionDays: '保留天数', range: '可配置范围：1-3650 天' },
    detail: { title: 'turn记录详情', headers: '响应 headers', body: '响应 body', truncatedHint: '该记录存在截断或未完整读取，请结合状态码和 headers 判断。' },
  },
}
