<template>
  <teleport to="body">
    <div class="picker-backdrop" @click.self="close">
      <div
        ref="dialogEl"
        class="picker"
        role="dialog"
        aria-modal="true"
        :aria-label="t('settings.pickFolder')"
        tabindex="-1"
      >
        <div class="picker-head">
          <h3>{{ t("settings.pickFolder") }}</h3>
          <button
            class="picker-icon"
            type="button"
            :aria-label="t('buttons.close')"
            @click="close"
          >
            <i class="material-icons">close</i>
          </button>
        </div>

        <!-- A plain div, not <nav>: the branding sheet turns every <nav> into
             the fixed sidebar. -->
        <div
          class="picker-crumbs"
          role="navigation"
          :aria-label="t('settings.pickFolder')"
        >
          <template v-for="(crumb, i) in crumbs" :key="crumb.path">
            <span v-if="i > 0" class="picker-crumb-sep">/</span>
            <button
              class="picker-crumb"
              type="button"
              :disabled="crumb.path === result?.path"
              @click="go(crumb.path)"
            >
              {{ crumb.label }}
            </button>
          </template>
        </div>

        <div ref="listEl" class="picker-list">
          <p v-if="loading" class="picker-note">{{ t("files.loading") }}</p>
          <p v-else-if="loadError" class="picker-note picker-note--error">
            {{ loadError }}
          </p>
          <template v-else-if="result">
            <button
              v-if="result.parent"
              class="picker-row picker-row--up"
              type="button"
              @click="go(result.parent)"
            >
              <i class="material-icons">arrow_upward</i>
              <span class="picker-row-name">{{ t("settings.parentFolder") }}</span>
            </button>

            <p v-if="result.entries.length === 0" class="picker-note">
              {{ t("settings.noSubfolders") }}
            </p>

            <button
              v-for="entry in result.entries"
              :key="entry.path"
              class="picker-row"
              type="button"
              @click="go(entry.path)"
            >
              <i class="material-icons">folder</i>
              <span class="picker-row-name">{{ entry.name }}</span>
              <span
                v-if="entry.mount"
                class="picker-tag"
                :class="{ 'picker-tag--ro': entry.readOnly }"
              >
                {{
                  entry.readOnly
                    ? t("settings.diskReadOnly")
                    : t("settings.diskMount")
                }}
              </span>
              <span v-if="entry.free" class="picker-row-meta">
                {{ prettyBytes(entry.free, { binary: true }) }}
              </span>
              <i class="material-icons picker-row-chevron">chevron_right</i>
            </button>
          </template>
        </div>

        <div class="picker-foot">
          <div class="picker-selection">
            <span class="picker-path">{{ result?.path || "…" }}</span>
            <span v-if="reasonText" class="picker-reason">{{ reasonText }}</span>
          </div>
          <div class="picker-actions">
            <button
              class="button button--flat button--grey"
              type="button"
              @click="close"
            >
              {{ t("buttons.cancel") }}
            </button>
            <button
              class="button"
              type="button"
              :disabled="!canSelect"
              @click="choose"
            >
              {{ t("settings.selectFolder") }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import prettyBytes from "pretty-bytes";

import { disks as api } from "@/api";

const props = defineProps<{
  // Where the picker opens. Empty starts at the filesystem root.
  initialPath?: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "select", path: string): void;
}>();

const { t } = useI18n();

const result = ref<IBrowseResult | null>(null);
const loading = ref(true);
const loadError = ref("");
const dialogEl = ref<HTMLElement | null>(null);
const listEl = ref<HTMLElement | null>(null);

const canSelect = computed(
  () => !loading.value && !!result.value?.selectable && !loadError.value
);

const reasonText = computed(() => {
  const key = reasonKey(result.value?.reason);
  return key ? t(key) : "";
});

function reasonKey(reason?: IBrowseEntry["reason"]) {
  switch (reason) {
    case "root":
      return "settings.reasonRoot";
    case "server-root":
      return "settings.reasonServerRoot";
    case "not-mount":
      return "settings.reasonNotMount";
    case "registered":
      return "settings.reasonRegistered";
    case "nested":
      return "settings.reasonNested";
    default:
      return "";
  }
}

const crumbs = computed(() => {
  const path = result.value?.path;
  if (!path) return [];

  const sep = path.includes("\\") ? "\\" : "/";
  const parts = path.split(sep).filter(Boolean);

  if (sep === "/") {
    const out = [{ label: "/", path: "/" }];
    let acc = "";
    for (const part of parts) {
      acc += "/" + part;
      out.push({ label: part, path: acc });
    }
    return out;
  }

  // Windows: the first segment is the volume, e.g. "C:".
  if (parts.length === 0) return [];
  const root = parts[0] + sep;
  const out = [{ label: root, path: root }];
  let acc = parts[0];
  for (const part of parts.slice(1)) {
    acc += sep + part;
    out.push({ label: part, path: acc });
  }
  return out;
});

const load = async (path: string) => {
  loading.value = true;
  loadError.value = "";
  try {
    result.value = await api.browse(path);
    if (listEl.value) listEl.value.scrollTop = 0;
  } catch (err: any) {
    loadError.value = err?.message || String(err);
  } finally {
    loading.value = false;
  }
};

const go = (path: string) => {
  if (path && path !== result.value?.path) load(path);
};

const close = () => emit("close");

const choose = () => {
  if (canSelect.value && result.value) emit("select", result.value.path);
};

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === "Escape") {
    event.stopPropagation();
    close();
  }
};

onMounted(() => {
  window.addEventListener("keydown", onKeydown);
  dialogEl.value?.focus();
  load(props.initialPath || "");
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", onKeydown);
});
</script>
