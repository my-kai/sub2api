import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { nextTick, reactive } from 'vue'
import AppHeader from '@/components/layout/AppHeader.vue'

const routerPushMock = vi.fn()
const routeState = reactive({
  name: 'Dashboard',
  meta: {},
  params: {},
})

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: routerPushMock,
  }),
  useRoute: () => routeState,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'profile.changePassword': 'Change Password',
        'nav.profile': 'Profile',
        'nav.apiKeys': 'API Keys',
        'nav.github': 'GitHub',
        'nav.docs': 'Docs',
        'nav.logout': 'Logout',
        'common.balance': 'Balance',
        'common.giftBalance': 'Gift Balance',
      }
      return translations[key] ?? key
    },
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    contactInfo: '',
    docUrl: '',
    cachedPublicSettings: {
      custom_menu_items: [],
    },
    toggleMobileSidebar: vi.fn(),
  }),
  useAuthStore: () => ({
    user: {
      email: 'user@example.com',
      username: 'tester',
      role: 'user',
      balance: 10,
      gift_balance: 2,
    },
    isAdmin: false,
    logout: vi.fn(),
  }),
  useOnboardingStore: () => ({
    replay: vi.fn(),
  }),
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    customMenuItems: [],
  }),
}))

vi.mock('@/components/common/LocaleSwitcher.vue', () => ({
  default: { template: '<div data-testid="locale-switcher" />' },
}))

vi.mock('@/components/common/SubscriptionProgressMini.vue', () => ({
  default: { template: '<div data-testid="subscription-progress" />' },
}))

vi.mock('@/components/common/AnnouncementBell.vue', () => ({
  default: { template: '<div data-testid="announcement-bell" />' },
}))

vi.mock('@/components/user/profile/ProfilePasswordForm.vue', () => ({
  default: {
    emits: ['success'],
    template: '<form data-testid="password-form" @submit.prevent="$emit(\'success\')" />',
  },
}))

describe('AppHeader password action', () => {
  it('opens password dialog instead of navigating to profile', async () => {
    const wrapper = mount(AppHeader, {
      attachTo: document.body,
      global: {
        stubs: {
          Teleport: true,
          RouterLink: {
            props: ['to'],
            template: '<a :href="to"><slot /></a>',
          },
        },
      },
    })

    await wrapper.get('button[aria-label="User Menu"]').trigger('click')
    await wrapper.get('button.dropdown-item').trigger('click')
    await nextTick()

    expect(routerPushMock).not.toHaveBeenCalledWith('/profile')
    expect(wrapper.find('[data-testid="password-form"]').exists()).toBe(true)

    wrapper.unmount()
  })
})
