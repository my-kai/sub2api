export default {
  turnLog: {
    title: 'Turn Records',
    description: 'Inspect and capture OpenAI OAuth upstream responses',
    empty: 'No turn records yet',
    truncated: 'Body truncated',
    complete: 'Body complete',
    partial: 'Body partially read',
    filters: { accountId: 'Account ID', status: 'Status code' },
    columns: { time: 'Time', account: 'Account', status: 'Status', payload: 'Response', headers: 'Response headers', body: 'Response body' },
    config: { retentionDays: 'Retention days', range: 'Allowed range: 1-3650 days' },
    detail: { title: 'Turn record detail', headers: 'Response headers', body: 'Response body', truncatedHint: 'This record was truncated or not fully read; use the status and headers for diagnosis.' },
    capture: { open: 'Capture state', title: 'Capture state', account: 'OpenAI OAuth account', accountPlaceholder: 'Select an account', model: 'Model', ip: 'Egress IP', accountDefaultIp: 'Use account configuration', directIp: 'Direct', submit: 'Extract', loading: 'Requesting', status: 'HTTP status', failed: 'Upstream request failed. Check the account, egress, and network.', loadAccountsFailed: 'Failed to load OpenAI OAuth accounts.', truncated: 'The response exceeded the display limit and was truncated.' },
  },
}
