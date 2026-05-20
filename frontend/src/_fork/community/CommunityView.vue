<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <!-- Page Header -->
      <header class="space-y-2">
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
          {{ t('community.title') }}
        </h1>
        <p class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('community.description') }}
        </p>
      </header>

      <!-- Cards Grid -->
      <div class="grid gap-6 lg:grid-cols-2">
        <!-- QQ 群卡片 -->
        <section class="card flex flex-col overflow-hidden">
          <header class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <div class="flex items-center gap-3">
              <span
                class="flex h-10 w-10 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-500/15 dark:text-primary-300"
              >
                <svg viewBox="0 0 24 24" fill="currentColor" class="h-5 w-5">
                  <path
                    d="M12 2C7.6 2 4 5.6 4 10c0 2.2.9 4.2 2.4 5.7-.2.8-.6 1.7-1.2 2.5-.2.3-.1.7.3.7 1.5 0 3-.6 4.1-1.5.8.3 1.7.4 2.4.4 4.4 0 8-3.6 8-8s-3.6-7.8-8-7.8z"
                  />
                </svg>
              </span>
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ t('community.qq.title') }}
                </h2>
                <p class="text-xs text-gray-500 dark:text-dark-400">
                  {{ t('community.qq.subtitle') }}
                </p>
              </div>
            </div>
            <span
              class="rounded-full bg-primary-50 px-2.5 py-1 text-[11px] font-medium text-primary-600 dark:bg-primary-500/15 dark:text-primary-300"
            >
              QQ
            </span>
          </header>

          <div class="flex flex-1 flex-col items-center justify-center px-6 py-8">
            <!-- 二维码本身作为 hero，不再套白底卡 -->
            <div
              class="mb-5 overflow-hidden rounded-2xl shadow-lg shadow-gray-900/5 ring-1 ring-gray-200/70 dark:shadow-black/40 dark:ring-dark-700"
            >
              <img
                :src="qqQrCode"
                alt="QQ Group QR Code"
                class="block h-auto w-[200px] select-none"
                loading="lazy"
                draggable="false"
              />
            </div>

            <h3 class="mb-1 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('community.qq.cta') }}
            </h3>
            <p class="mb-6 text-center text-sm text-gray-500 dark:text-dark-400">
              {{ t('community.qq.hint') }}
            </p>

            <!-- 群号 chip -->
            <div class="mb-4 w-full max-w-sm space-y-2">
              <p class="text-center text-xs uppercase tracking-wider text-gray-400 dark:text-dark-500">
                {{ t('community.qq.groupNumberLabel') }}
              </p>
              <button
                type="button"
                class="group flex w-full items-center justify-between rounded-xl border border-gray-200 bg-gray-50 px-4 py-2.5 transition hover:border-primary-300 hover:bg-primary-50 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-500/40 dark:hover:bg-primary-500/10"
                :title="t('community.qq.copyTooltip')"
                @click="copyGroupNumber"
              >
                <span class="font-mono text-base font-semibold tracking-wider text-gray-900 dark:text-white">
                  {{ QQ_GROUP_NUMBER }}
                </span>
                <Icon
                  :name="copied ? 'check' : 'copy'"
                  size="md"
                  :class="copied ? 'text-emerald-500' : 'text-gray-400 transition group-hover:text-primary-500 dark:text-dark-500'"
                />
              </button>
            </div>

            <button
              type="button"
              class="btn btn-primary w-full max-w-sm gap-2"
              @click="copyGroupNumber"
            >
              <Icon :name="copied ? 'check' : 'copy'" size="md" />
              {{ copied ? t('community.qq.copied') : t('community.qq.copyButton') }}
            </button>
          </div>
        </section>

        <!-- Telegram 群卡片 -->
        <section class="card flex flex-col overflow-hidden">
          <header class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <div class="flex items-center gap-3">
              <span
                class="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-50 text-sky-600 dark:bg-sky-500/15 dark:text-sky-300"
              >
                <svg viewBox="0 0 24 24" fill="currentColor" class="h-5 w-5">
                  <path
                    d="M21.5 4.3 18.2 19.7c-.2 1.1-.9 1.4-1.8.9l-5-3.7-2.4 2.3c-.3.3-.5.5-1 .5l.4-5.1 9.3-8.4c.4-.4-.1-.6-.6-.2l-11.5 7.2-4.9-1.5c-1.1-.3-1.1-1.1.2-1.6l19.3-7.4c.9-.3 1.7.2 1.3 1.6z"
                  />
                </svg>
              </span>
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ t('community.telegram.title') }}
                </h2>
                <p class="text-xs text-gray-500 dark:text-dark-400">
                  {{ t('community.telegram.subtitle') }}
                </p>
              </div>
            </div>
            <span
              class="rounded-full bg-sky-50 px-2.5 py-1 text-[11px] font-medium text-sky-600 dark:bg-sky-500/15 dark:text-sky-300"
            >
              Telegram
            </span>
          </header>

          <div class="flex flex-1 flex-col items-center justify-center px-6 py-8">
            <div
              class="mb-5 flex h-[200px] w-[200px] items-center justify-center rounded-2xl bg-gradient-to-br from-sky-400 to-sky-600 text-white shadow-lg shadow-sky-500/20"
            >
              <svg viewBox="0 0 24 24" fill="currentColor" class="h-24 w-24">
                <path
                  d="M21.5 4.3 18.2 19.7c-.2 1.1-.9 1.4-1.8.9l-5-3.7-2.4 2.3c-.3.3-.5.5-1 .5l.4-5.1 9.3-8.4c.4-.4-.1-.6-.6-.2l-11.5 7.2-4.9-1.5c-1.1-.3-1.1-1.1.2-1.6l19.3-7.4c.9-.3 1.7.2 1.3 1.6z"
                />
              </svg>
            </div>

            <h3 class="mb-1 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('community.telegram.cta') }}
            </h3>
            <p class="mb-6 text-center text-sm text-gray-500 dark:text-dark-400">
              {{ t('community.telegram.hint') }}
            </p>

            <!-- Link chip -->
            <div class="mb-4 w-full max-w-sm space-y-2">
              <p class="text-center text-xs uppercase tracking-wider text-gray-400 dark:text-dark-500">
                {{ t('community.telegram.linkLabel') }}
              </p>
              <button
                type="button"
                class="group flex w-full items-center justify-between rounded-xl border border-gray-200 bg-gray-50 px-4 py-2.5 transition hover:border-sky-300 hover:bg-sky-50 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-sky-500/40 dark:hover:bg-sky-500/10"
                :title="t('community.telegram.copyTooltip')"
                @click="copyTelegramLink"
              >
                <span class="truncate font-mono text-sm text-gray-700 dark:text-dark-200">
                  {{ TELEGRAM_LINK }}
                </span>
                <Icon
                  :name="tgCopied ? 'check' : 'copy'"
                  size="md"
                  :class="tgCopied ? 'text-emerald-500' : 'text-gray-400 transition group-hover:text-sky-500 dark:text-dark-500'"
                />
              </button>
            </div>

            <a
              :href="TELEGRAM_LINK"
              target="_blank"
              rel="noopener noreferrer"
              class="btn btn-primary w-full max-w-sm gap-2"
            >
              <Icon name="externalLink" size="md" />
              {{ t('community.telegram.joinButton') }}
            </a>
          </div>
        </section>
      </div>

      <!-- Note -->
      <div class="card p-5">
        <div class="flex items-start gap-3">
          <span
            class="mt-0.5 flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-amber-50 text-amber-600 dark:bg-amber-500/15 dark:text-amber-300"
          >
            <Icon name="lightbulb" size="md" />
          </span>
          <div class="space-y-1">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('community.note.title') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-400">
              {{ t('community.note.body') }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import qqQrCode from './assets/qq-group-qrcode.jpg'

const QQ_GROUP_NUMBER = '368511092'
const TELEGRAM_LINK = 'https://t.me/+5b3HXcOiiHNhMDE9'

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const copied = ref(false)
const tgCopied = ref(false)

async function copyGroupNumber() {
  const ok = await copyToClipboard(QQ_GROUP_NUMBER, t('community.qq.copySuccess'))
  if (ok) {
    copied.value = true
    setTimeout(() => (copied.value = false), 1800)
  }
}

async function copyTelegramLink() {
  const ok = await copyToClipboard(TELEGRAM_LINK, t('community.telegram.copySuccess'))
  if (ok) {
    tgCopied.value = true
    setTimeout(() => (tgCopied.value = false), 1800)
  }
}
</script>
