import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import audit from './audit'
import promptAudit from './promptAudit'
import promptAuditV2 from './promptAuditV2'
import turnLog from './turnLog'
import plugins from './plugins'

export default {
  ...overview,
  ...channels,
  ...accounts,
  ...resources,
  ...ops,
  ...settings,
  ...audit,
  ...promptAudit,
  ...promptAuditV2,
  ...turnLog,
  ...plugins,
}
