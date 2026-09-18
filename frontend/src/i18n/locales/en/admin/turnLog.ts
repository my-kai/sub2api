export default {
  turnLog: {
    title: 'Turn Records',
    description: 'Inspect OpenAI OAuth upstream error responses',
    empty: 'No turn records yet',
    truncated: 'Body truncated',
    complete: 'Body complete',
    partial: 'Body partially read',
    filters: { accountId: 'Account ID', status: 'Status code' },
    columns: { time: 'Time', account: 'Account', status: 'Status', payload: 'Response', headers: 'Response headers', body: 'Response body' },
    config: { retentionDays: 'Retention days', range: 'Allowed range: 1-3650 days' },
    detail: { title: 'Turn record detail', headers: 'Response headers', body: 'Response body', truncatedHint: 'This record was truncated or not fully read; use the status and headers for diagnosis.' },
  },
}
