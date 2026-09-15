<template>
  <AppLayout>
    <div class="mx-auto max-w-[1500px] px-4 py-6 sm:px-6 lg:px-8">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.promptAuditV2.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.promptAuditV2.description') }}</p>
        </div>
        <div class="tabs" role="tablist" :aria-label="t('admin.promptAuditV2.title')">
          <button
            type="button"
            role="tab"
            class="tab"
            :class="{ 'tab-active': activeTab === 'config' }"
            :aria-selected="activeTab === 'config'"
            aria-controls="prompt-audit-v2-config-panel"
            @click="activeTab = 'config'"
          >
            {{ t('admin.promptAuditV2.tabs.config') }}
          </button>
          <button
            type="button"
            role="tab"
            class="tab"
            :class="{ 'tab-active': activeTab === 'events' }"
            :aria-selected="activeTab === 'events'"
            aria-controls="prompt-audit-v2-events-panel"
            @click="activeTab = 'events'"
          >
            {{ t('admin.promptAuditV2.tabs.events') }}
          </button>
        </div>
      </div>

      <div v-if="pageError" role="alert" class="mt-5 border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300">{{ pageError }}</div>
      <div v-if="loadingConfig && !draft" class="card mt-5 py-16 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>

      <main v-else-if="draft" class="card mt-5 px-4 sm:px-6 lg:px-8">
        <div
          v-if="activeTab === 'config'"
          id="prompt-audit-v2-config-panel"
          key="config"
          role="tabpanel"
        >
          <RuntimePanel :runtime="runtime" :endpoints="draft.endpoints" :loading="loadingRuntime" @refresh="loadRuntime" />
          <ConfigPanel v-model="draft" @test="openPromptTest" />
          <EndpointPanel v-model="draft.endpoints" :probe-results="probeResults" :probing-ids="probingIds" @probe="probeEndpoint" />
          <RulePanel v-model="draft.rule" />
          <div class="sticky bottom-0 z-10 flex items-center justify-between border-t border-gray-200 bg-white/95 py-4 backdrop-blur dark:border-dark-700 dark:bg-dark-800/95">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ dirty ? t('admin.promptAuditV2.unsaved') : t('admin.promptAuditV2.saved') }}</span>
            <button type="button" class="btn btn-primary" :disabled="saving || !dirty" @click="saveConfig">{{ saving ? t('common.saving') : t('common.save') }}</button>
          </div>
        </div>

        <div
          v-else
          id="prompt-audit-v2-events-panel"
          key="events"
          role="tabpanel"
        >
          <EventPanel
            v-model="filters"
            :protocols="draft.protocols"
            :events="events"
            :total="eventTotal"
            :page="eventPage"
            :page-size="eventPageSize"
            :loading="loadingEvents"
            @search="searchEvents"
            @reset="resetEvents"
            @page="changeEventPage"
            @page-size="changeEventPageSize"
            @view="openEvent"
          />
        </div>
      </main>

      <EventDetailDialog :show="detailOpen" :loading="loadingDetail" :event="activeEvent" @close="closeEvent" />
      <PromptTestDialog
        :show="promptTestOpen"
        :loading="testingPrompt"
        :result="promptTestResult"
        :error-text="promptTestError"
        :validation-error="promptTestValidationError"
        @close="closePromptTest"
        @reset="resetPromptTestOutcome"
        @submit="runPromptTest"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import ConfigPanel from './components/ConfigPanel.vue'
import EndpointPanel from './components/EndpointPanel.vue'
import EventDetailDialog from './components/EventDetailDialog.vue'
import EventPanel from './components/EventPanel.vue'
import PromptTestDialog from './components/PromptTestDialog.vue'
import RulePanel from './components/RulePanel.vue'
import RuntimePanel from './components/RuntimePanel.vue'
import {
  getPromptAuditV2Config,
  getPromptAuditV2Event,
  getPromptAuditV2Runtime,
  listPromptAuditV2Events,
  probePromptAuditV2Endpoint,
  testPromptAuditV2Prompt,
  updatePromptAuditV2Config,
} from './api'
import type {
  PromptAuditV2Config,
  PromptAuditV2Endpoint,
  PromptAuditV2Event,
  PromptAuditV2PromptTestResult,
  PromptAuditV2ProbeResult,
  PromptAuditV2Runtime,
} from './types'
import { buildPromptAuditV2Update, clonePromptAuditV2Config, emptyPromptAuditV2Filters } from './viewModel'

const { t } = useI18n()
const appStore = useAppStore()
const activeTab = ref<'config' | 'events'>('config')
const serverConfig = ref<PromptAuditV2Config | null>(null)
const draft = ref<PromptAuditV2Config | null>(null)
const runtime = ref<PromptAuditV2Runtime | null>(null)
const filters = ref(emptyPromptAuditV2Filters())
const events = ref<PromptAuditV2Event[]>([])
const eventTotal = ref(0)
const eventPage = ref(1)
const eventPageSize = ref(20)
const activeEvent = ref<PromptAuditV2Event | null>(null)
const detailOpen = ref(false)
const promptTestOpen = ref(false)
const promptTestResult = ref<PromptAuditV2PromptTestResult | null>(null)
const promptTestError = ref('')
const probeResults = ref<Record<string, PromptAuditV2ProbeResult>>({})
const probingIds = ref<string[]>([])
const loadingConfig = ref(false)
const loadingRuntime = ref(false)
const loadingEvents = ref(false)
const loadingDetail = ref(false)
const testingPrompt = ref(false)
const saving = ref(false)
const pageError = ref('')
const dirty = computed(() => JSON.stringify(draft.value) !== JSON.stringify(serverConfig.value))
const promptTestValidationError = computed(() => {
  if (!draft.value) return t('admin.promptAuditV2.errors.loadConfig')
  const placeholderCount = draft.value.prompt_template.split('{{user_input}}').length - 1
  if (placeholderCount !== 1) return t('admin.promptAuditV2.promptTest.invalidPrompt')
  if (!draft.value.endpoints.some((endpoint) => endpoint.enabled)) {
    return t('admin.promptAuditV2.promptTest.noEnabledEndpoint')
  }
  return ''
})

onMounted(async () => {
  await loadConfig()
  await Promise.all([loadRuntime(), loadEvents()])
})

/** Loads the authoritative configuration and replaces both saved and draft state. */
async function loadConfig(): Promise<void> {
  loadingConfig.value = true
  pageError.value = ''
  try {
    const config = await getPromptAuditV2Config()
    serverConfig.value = clonePromptAuditV2Config(config)
    draft.value = clonePromptAuditV2Config(config)
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('admin.promptAuditV2.errors.loadConfig'))
  } finally {
    loadingConfig.value = false
  }
}

/** Saves the full draft and adopts the returned configuration version. */
async function saveConfig(): Promise<void> {
  if (!draft.value) return
  const validationError = validateDraftForSave(draft.value)
  if (validationError) {
    pageError.value = validationError
    appStore.showError(validationError)
    return
  }
  saving.value = true
  pageError.value = ''
  try {
    const saved = await updatePromptAuditV2Config(buildPromptAuditV2Update(draft.value))
    serverConfig.value = clonePromptAuditV2Config(saved)
    draft.value = clonePromptAuditV2Config(saved)
    appStore.showSuccess(t('admin.promptAuditV2.messages.saved'))
    await loadRuntime()
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('admin.promptAuditV2.errors.save'))
    appStore.showError(pageError.value)
  } finally {
    saving.value = false
  }
}

/**
 * Mirrors the server-side replacement checks so a generic invalid-config
 * response is not the first feedback for a missing field in an enabled draft.
 * The server remains authoritative; this only prevents a request that can be
 * diagnosed locally and keeps the displayed message tied to user-facing data.
 */
function validateDraftForSave(config: PromptAuditV2Config): string {
  if (config.enabled) {
    if (config.prompt_template.split('{{user_input}}').length - 1 !== 1) {
      return t('admin.promptAuditV2.validation.prompt')
    }
    if (config.worker_count < 1 || config.worker_count > 64) {
      return t('admin.promptAuditV2.validation.workers')
    }
    if (config.queue_capacity < 1 || config.queue_capacity > 10000) {
      return t('admin.promptAuditV2.validation.queue')
    }
    if (config.log_retention_days < 1 || config.log_retention_days > 3650) {
      return t('admin.promptAuditV2.validation.retention')
    }
    if (config.enabled_protocols.length === 0) {
      return t('admin.promptAuditV2.validation.protocol')
    }
    if (!config.endpoints.some((endpoint) => endpoint.enabled)) {
      return t('admin.promptAuditV2.validation.endpoint')
    }
  }

  for (const endpoint of config.endpoints) {
    if (!endpoint.name.trim() || !endpoint.model.trim() || !endpoint.base_url.trim()) {
      return t('admin.promptAuditV2.validation.endpointFields')
    }
    try {
      const parsedURL = new URL(endpoint.base_url.trim())
      if ((parsedURL.protocol !== 'http:' && parsedURL.protocol !== 'https:') || parsedURL.search || parsedURL.hash) {
        return t('admin.promptAuditV2.validation.endpointURL')
      }
    } catch {
      return t('admin.promptAuditV2.validation.endpointURL')
    }
    if (endpoint.priority < -100000 || endpoint.priority > 100000) {
      return t('admin.promptAuditV2.validation.endpointPriority')
    }
    if (endpoint.timeout_ms < 100 || endpoint.timeout_ms > 120000) {
      return t('admin.promptAuditV2.validation.endpointTimeout')
    }
    if (config.enabled && endpoint.enabled && !endpoint.has_api_key && !endpoint.api_key?.trim()) {
      return t('admin.promptAuditV2.validation.endpointKey')
    }
  }

  const rule = config.rule
  if (rule.confidence_threshold < 0 || rule.confidence_threshold > 1) {
    return t('admin.promptAuditV2.validation.ruleConfidence')
  }
  if (rule.window_minutes < 1 || rule.window_minutes > 10080 || rule.trigger_count < 1 || rule.trigger_count > 10000) {
    return t('admin.promptAuditV2.validation.ruleWindow')
  }
  if (rule.action === 'warning' && (!rule.restriction_minutes || rule.restriction_minutes < 1 || rule.restriction_minutes > 43200)) {
    return t('admin.promptAuditV2.validation.ruleRestriction')
  }
  if (rule.action === 'ban' && rule.restriction_minutes != null) {
    return t('admin.promptAuditV2.validation.ruleBanRestriction')
  }
  if (config.enabled && config.log_retention_days * 24 * 60 < rule.window_minutes) {
    return t('admin.promptAuditV2.validation.retentionWindow')
  }
  return ''
}

/** Refreshes non-secret process and queue counters. */
async function loadRuntime(): Promise<void> {
  loadingRuntime.value = true
  try {
    runtime.value = await getPromptAuditV2Runtime()
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('admin.promptAuditV2.errors.loadRuntime'))
  } finally {
    loadingRuntime.value = false
  }
}

/** Tests one endpoint and retains the result by stable endpoint ID. */
async function probeEndpoint(endpoint: PromptAuditV2Endpoint): Promise<void> {
  probingIds.value = [...probingIds.value, endpoint.id]
  try {
    probeResults.value = { ...probeResults.value, [endpoint.id]: await probePromptAuditV2Endpoint(endpoint) }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.promptAuditV2.errors.probe')))
  } finally {
    probingIds.value = probingIds.value.filter((id) => id !== endpoint.id)
  }
}

/** Opens a fresh test dialog while retaining the current unsaved page draft. */
function openPromptTest(): void {
  resetPromptTestOutcome()
  promptTestOpen.value = true
}

/** Removes an outcome that no longer belongs to the current test input. */
function resetPromptTestOutcome(): void {
  promptTestResult.value = null
  promptTestError.value = ''
}

/** Closes the test dialog after any active model call finishes. */
function closePromptTest(): void {
  if (testingPrompt.value) return
  promptTestOpen.value = false
  promptTestResult.value = null
  promptTestError.value = ''
}

/** Calls only the configured audit model services and keeps the result inside the dialog. */
async function runPromptTest(userInput: string): Promise<void> {
  if (!draft.value || promptTestValidationError.value || !userInput.trim()) return
  testingPrompt.value = true
  promptTestResult.value = null
  promptTestError.value = ''
  try {
    promptTestResult.value = await testPromptAuditV2Prompt({
      prompt_template: draft.value.prompt_template,
      user_input: userInput,
      endpoints: draft.value.endpoints.map((endpoint) => ({ ...endpoint })),
    })
  } catch (error) {
    promptTestError.value = extractApiErrorMessage(error, t('admin.promptAuditV2.errors.testPrompt'))
  } finally {
    testingPrompt.value = false
  }
}

/** Lists hit events without requesting complete messages. */
async function loadEvents(): Promise<void> {
  loadingEvents.value = true
  try {
    const requestFilters = {
      ...filters.value,
      start_at: toRFC3339(filters.value.start_at),
      end_at: toRFC3339(filters.value.end_at),
    }
    const result = await listPromptAuditV2Events(requestFilters, eventPage.value, eventPageSize.value)
    events.value = result.items
    eventTotal.value = result.total
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('admin.promptAuditV2.errors.loadEvents'))
  } finally {
    loadingEvents.value = false
  }
}

function searchEvents(): void {
  eventPage.value = 1
  void loadEvents()
}

function resetEvents(): void {
  filters.value = emptyPromptAuditV2Filters()
  searchEvents()
}

function changeEventPage(page: number): void {
  eventPage.value = page
  void loadEvents()
}

function changeEventPageSize(pageSize: number): void {
  eventPageSize.value = pageSize
  eventPage.value = 1
  void loadEvents()
}

/** Loads complete message content only after the administrator opens a row. */
async function openEvent(id: number): Promise<void> {
  detailOpen.value = true
  activeEvent.value = null
  loadingDetail.value = true
  try {
    activeEvent.value = await getPromptAuditV2Event(id)
  } catch (error) {
    pageError.value = extractApiErrorMessage(error, t('admin.promptAuditV2.errors.loadDetail'))
    detailOpen.value = false
  } finally {
    loadingDetail.value = false
  }
}

function closeEvent(): void {
  detailOpen.value = false
  activeEvent.value = null
}

function toRFC3339(value: string): string {
  return value ? new Date(value).toISOString() : ''
}
</script>
