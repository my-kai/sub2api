<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-5">
      <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <Icon name="gift" size="lg" class="text-red-500" />
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">活动管理</h1>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">管理红包雨活动。</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button type="button" class="btn btn-secondary" :disabled="listLoading" @click="loadActivities">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': listLoading }" />
            <span>刷新</span>
          </button>
          <button type="button" class="btn btn-primary" @click="openCreateForm">
            <Icon name="plus" size="sm" />
            <span>新建活动</span>
          </button>
        </div>
      </header>

      <div v-if="errorMessage" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <AdminActivityListPanel
        :activities="activities"
        :loading="listLoading"
        :action-loading="actionLoading"
        @select="selectActivity"
        @edit="openEditForm"
        @action="openActionConfirm"
      />

      <section class="grid gap-5 xl:grid-cols-[minmax(0,1fr)_420px]">
        <AdminActivityDetailPanel
          :activity="selectedActivity"
          :loading="detailLoading"
          @reload="reloadSelectedActivity"
        />

        <AdminActivityClaimsPanel
          :activity="selectedActivity"
          :claims="claims"
          :loading="claimsLoading"
          :page="claimsPage"
          :page-size="claimsPageSize"
          :total="claimsTotal"
          @reload="loadClaims"
          @update:page="handleClaimsPageChange"
          @update:pageSize="handleClaimsPageSizeChange"
        />
      </section>
    </div>

    <BaseDialog :show="formOpen" :title="editingActivity ? '编辑活动' : '新建活动'" width="extra-wide" @close="closeForm">
      <AdminRedPacketRainForm
        form-id="admin-red-packet-rain-form"
        :activity="editingActivity"
        :saving="saving"
        @submit="handleSaveActivity"
      />
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeForm">取消</button>
          <button type="submit" form="admin-red-packet-rain-form" class="btn btn-primary" :disabled="formSaveDisabled">
            <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
            <Icon v-else name="check" size="sm" />
            <span>保存</span>
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="Boolean(pendingAction)"
      :title="pendingActionTitle"
      :message="pendingActionMessage"
      confirm-text="确认"
      cancel-text="取消"
      danger
      @confirm="confirmPendingAction"
      @cancel="pendingAction = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import AdminActivityClaimsPanel from '../components/AdminActivityClaimsPanel.vue'
import AdminActivityDetailPanel from '../components/AdminActivityDetailPanel.vue'
import AdminActivityListPanel from '../components/AdminActivityListPanel.vue'
import AdminRedPacketRainForm from '../components/AdminRedPacketRainForm.vue'
import { useAdminActivityManagement } from '../components/useAdminActivityManagement'

const {
  activities,
  actionLoading,
  claims,
  claimsLoading,
  claimsPage,
  claimsPageSize,
  claimsTotal,
  closeForm,
  confirmPendingAction,
  detailLoading,
  editingActivity,
  errorMessage,
  formOpen,
  formSaveDisabled,
  handleClaimsPageChange,
  handleClaimsPageSizeChange,
  handleSaveActivity,
  listLoading,
  loadActivities,
  loadClaims,
  openActionConfirm,
  openCreateForm,
  openEditForm,
  pendingAction,
  pendingActionMessage,
  pendingActionTitle,
  reloadSelectedActivity,
  saving,
  selectActivity,
  selectedActivity,
} = useAdminActivityManagement()
</script>
