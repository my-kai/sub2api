<template>
  <form :id="formId" class="space-y-5" @submit.prevent="handleSubmit">
    <div v-if="locked" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
      活动已开始，不可编辑
    </div>

    <div v-if="errors.length > 0" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
      <p class="font-medium">请检查表单</p>
      <ul class="mt-2 list-disc space-y-1 pl-5">
        <li v-for="error in errors" :key="error">{{ error }}</li>
      </ul>
    </div>

    <section class="space-y-4">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">基础信息</h3>
      <div class="grid gap-4 md:grid-cols-2">
        <Input v-model="form.title" label="活动标题" :disabled="locked || saving" required />
        <Input v-model="form.cover_url" label="封面地址" :disabled="locked || saving" />
      </div>
      <TextArea v-model="form.description" label="活动描述" :disabled="locked || saving" :rows="3" />
      <div class="grid gap-4 md:grid-cols-2">
        <Input v-model="form.starts_at" type="datetime-local" label="开始时间" :disabled="locked || saving" required />
        <Input v-model="form.ends_at" type="datetime-local" label="结束时间" :disabled="locked || saving" required />
      </div>
    </section>

    <section class="space-y-4">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">轮次配置</h3>
      <div class="grid gap-4 md:grid-cols-3">
        <Input v-model="form.round_count" type="number" label="轮数" :disabled="locked || saving" required />
        <Input v-model="form.round_duration_seconds" type="number" label="单轮时长秒" :disabled="locked || saving" required />
        <Input v-model="form.round_interval_seconds" type="number" label="轮次间隔秒" :disabled="locked || saving" required />
      </div>
    </section>

    <section class="space-y-4">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">奖励规则</h3>
      <div class="grid gap-4 md:grid-cols-2">
        <Input v-model="form.total_budget" label="活动总预算" :disabled="locked || saving" required />
        <Input v-model="form.per_user_total_cap" label="单用户活动总上限" :disabled="locked || saving" required />
        <Input v-model="form.per_user_round_cap" label="单用户单轮上限" :disabled="locked || saving" required />
        <Input v-model="form.max_single_reward" label="单次最高奖励" :disabled="locked || saving" required />
        <Input v-model="form.base_unit_amount" label="基础奖励金额" :disabled="locked || saving" required />
        <Input v-model="form.probability_step" label="概率步长" :disabled="locked || saving" required />
        <Input v-model="form.gift_validity_days" type="number" label="赠送余额有效天数" :disabled="locked || saving" required />
      </div>
    </section>
  </form>
</template>

<script setup lang="ts">
import { reactive, ref, watch, computed } from 'vue'
import Input from '@/components/common/Input.vue'
import TextArea from '@/components/common/TextArea.vue'
import type { AdminCustomActivityDetail, AdminCustomActivityUpsertRequest } from '../types'
import {
  buildRedPacketRainPayload,
  createDefaultRedPacketRainForm,
  isEditableActivityStatus,
  redPacketRainFormFromActivity,
  validateRedPacketRainForm,
} from './adminRedPacketRainFormModel'

const props = withDefaults(defineProps<{
  formId: string
  activity?: AdminCustomActivityDetail | null
  saving?: boolean
}>(), {
  activity: null,
  saving: false,
})

const emit = defineEmits<{
  submit: [payload: AdminCustomActivityUpsertRequest]
}>()

const errors = ref<string[]>([])
const form = reactive(createDefaultRedPacketRainForm())
const locked = computed(() => Boolean(props.activity && !isEditableActivityStatus(props.activity.status)))

watch(
  () => props.activity,
  (activity) => {
    Object.assign(form, activity ? redPacketRainFormFromActivity(activity) : createDefaultRedPacketRainForm())
    errors.value = []
  },
  { immediate: true },
)

/**
 * 提交前先在前端拦截明显错误。
 *
 * 这里仅校验格式和大小关系，奖励金额仍由服务端按实际领取结果计算。
 */
function handleSubmit(): void {
  errors.value = validateRedPacketRainForm(form)
  if (errors.value.length > 0 || locked.value) {
    return
  }
  emit('submit', buildRedPacketRainPayload(form))
}
</script>
