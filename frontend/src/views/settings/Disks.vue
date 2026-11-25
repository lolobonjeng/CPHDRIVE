<template>
  <errors v-if="error" :errorCode="error.status" />
  <div class="row" v-else-if="!layoutStore.loading">
    <div class="column column--wide">
      <div class="card">
        <div class="card-title">
          <h2>{{ t("settings.disks") }}</h2>
          <div class="disk-toolbar">
            <button
              class="button button--flat button--grey"
              type="button"
              :disabled="busy"
              @click="openDetect"
            >
              {{ t("settings.detectDisks") }}
            </button>
            <button class="button" type="button" @click="startAdd">
              {{ t("settings.addDisk") }}
            </button>
          </div>
        </div>

        <div class="card-content full">
          <p class="small disk-intro">{{ t("settings.disksHelp") }}</p>

          <form v-if="form.open" class="disk-form" @submit.prevent="submitForm">
            <div class="disk-form-row">
              <label>
                <span>{{ t("settings.diskName") }}</span>
                <input
                  class="input"
                  v-model.trim="form.name"
                  :placeholder="t('settings.diskNamePlaceholder')"
                />
              </label>
              <label class="disk-form-path">
                <span>{{ t("settings.diskPath") }}</span>
                <div class="disk-path-field">
                  <span
                    class="disk-path-value"
                    :class="{ 'disk-path-value--empty': !form.path }"
                  >
                    {{ form.path || t("settings.noFolderChosen") }}
                  </span>
                  <button
                    type="button"
                    class="button button--flat button--grey"
                    @click="picker.open = true"
                  >
                    {{ t("settings.browse") }}
                  </button>
                </div>
              </label>
              <div class="disk-form-actions">
                <button
                  type="button"
                  class="button button--flat button--grey"
                  @click="cancelForm"
                >
                  {{ t("buttons.cancel") }}
                </button>
                <button type="submit" class="button" :disabled="busy">
                  {{ t("buttons.save") }}
                </button>
              </div>
            </div>
            <p class="small disk-form-error" v-if="form.error">
              {{ form.error }}
            </p>
          </form>

          <div v-if="detect.open" class="disk-detect">
            <h3>{{ t("settings.detectedDisks") }}</h3>
            <p class="small" v-if="detect.loading">{{ t("files.loading") }}</p>
            <p class="small" v-else-if="candidates.length === 0">
              {{ t("settings.noDetectedDisks") }}
            </p>
            <div v-else class="disk-detect-list">
              <button
                v-for="candidate in candidates"
                :key="candidate.path"
                class="disk-detect-row"
                type="button"
                @click="useCandidate(candidate)"
              >
                <span class="disk-detect-name">{{ candidate.name }}</span>
                <span class="disk-detect-path">{{ candidate.path }}</span>
                <span class="disk-detect-meta">
                  {{ prettyBytes(candidate.free, { binary: true }) }}
                  {{ t("settings.free") }}
                </span>
              </button>
            </div>
            <button
              class="button button--flat"
              type="button"
              @click="detect.open = false"
            >
              {{ t("buttons.close") }}
            </button>
          </div>

          <div class="disks-grid">
            <article
              v-for="disk in disks"
              :key="disk.id"
              class="disk-card"
              :class="{ 'disk-card--offline': !disk.online }"
            >
              <div class="disk-card-head">
                <div>
                  <h3>{{ disk.name }}</h3>
                  <p class="disk-path">{{ disk.path }}</p>
                </div>
                <span
                  class="disk-badge"
                  :class="
                    disk.virtual
                      ? 'disk-badge--system'
                      : disk.online
                        ? 'disk-badge--online'
                        : 'disk-badge--offline'
                  "
                >
                  {{
                    disk.virtual
                      ? t("settings.systemDisk")
                      : disk.online
                        ? t("settings.online")
                        : t("settings.offline")
                  }}
                </span>
              </div>

              <template v-if="disk.online">
                <progress-bar
                  :val="usedPercent(disk)"
                  size="small"
                  :bar-color="barColor(disk)"
                  :bar-border-radius="4"
                ></progress-bar>
                <div class="disk-stats">
                  <span>
                    {{
                      t("settings.diskUsedOf", {
                        used: prettyBytes(disk.used, { binary: true }),
                        total: prettyBytes(disk.total, { binary: true }),
                      })
                    }}
                  </span>
                  <span>
                    {{ prettyBytes(disk.free, { binary: true }) }}
                    {{ t("settings.free") }}
                  </span>
                </div>
              </template>
              <p v-else class="disk-offline-note">
                {{ t("settings.diskOfflineHelp") }}
              </p>

              <footer class="disk-card-foot">
                <span class="disk-users">
                  {{
                    t(
                      disk.userCount === 1
                        ? "settings.diskUserOne"
                        : "settings.diskUsers",
                      { count: disk.userCount }
                    )
                  }}
                </span>
                <div class="disk-actions" v-if="!disk.virtual">
                  <button
                    class="button button--flat button--grey"
                    type="button"
                    @click="startEdit(disk)"
                  >
                    {{ t("buttons.edit") }}
                  </button>
                  <button
                    class="button button--flat button--red"
                    type="button"
                    @click="confirmDelete = disk.id"
                  >
                    {{ t("buttons.delete") }}
                  </button>
                </div>
              </footer>

              <div v-if="confirmDelete === disk.id" class="disk-confirm">
                <span>
                  {{ t("settings.deleteDiskConfirm", { name: disk.name }) }}
                </span>
                <button
                  class="button button--flat button--grey"
                  type="button"
                  @click="confirmDelete = null"
                >
                  {{ t("buttons.cancel") }}
                </button>
                <button
                  class="button button--red"
                  type="button"
                  :disabled="busy"
                  @click="removeDisk(disk)"
                >
                  {{ t("buttons.delete") }}
                </button>
              </div>
            </article>
          </div>
        </div>
      </div>
    </div>
  </div>

  <disk-path-picker
    v-if="picker.open"
    :initial-path="form.path"
    @close="picker.open = false"
    @select="applyPickedPath"
  />
</template>

<script setup lang="ts">
import { inject, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import prettyBytes from "pretty-bytes";

import { disks as api } from "@/api";
import { StatusError } from "@/api/utils";
import { useLayoutStore } from "@/stores/layout";
import ProgressBar from "@/components/ProgressBar.vue";
import DiskPathPicker from "@/components/settings/DiskPathPicker.vue";
import Errors from "@/views/Errors.vue";

const { t } = useI18n();
const layoutStore = useLayoutStore();
const $showError = inject<IToastError>("$showError")!;

const error = ref<StatusError | null>(null);
const disks = ref<IDisk[]>([]);
const candidates = ref<IDiskCandidate[]>([]);
const busy = ref(false);
const confirmDelete = ref<number | null>(null);

const form = reactive({
  open: false,
  id: 0,
  name: "",
  path: "",
  error: "",
});

const detect = reactive({
  open: false,
  loading: false,
});

const picker = reactive({
  open: false,
});

const usedPercent = (disk: IDisk) => {
  if (disk.total === 0) return 0;
  return Math.min(100, Math.round((disk.used / disk.total) * 100));
};

const barColor = (disk: IDisk) => {
  if (disk.total === 0) return "#0a84ff";
  return disk.used / disk.total > 0.9 ? "#ff453a" : "#0a84ff";
};

const load = async () => {
  layoutStore.loading = true;
  try {
    disks.value = await api.getAll();
    error.value = null;
  } catch (err) {
    if (err instanceof StatusError) {
      error.value = err;
    }
  } finally {
    layoutStore.loading = false;
  }
};

onMounted(load);

const startAdd = () => {
  form.open = true;
  form.id = 0;
  form.name = "";
  form.path = "";
  form.error = "";
};

const startEdit = (disk: IDisk) => {
  form.open = true;
  form.id = disk.id;
  form.name = disk.name;
  form.path = disk.path;
  form.error = "";
};

const cancelForm = () => {
  form.open = false;
  form.error = "";
};

const applyPickedPath = (path: string) => {
  form.path = path;
  picker.open = false;
  form.error = "";

  // Only name a brand-new disk, and only when the admin has not typed one yet.
  if (form.id === 0 && !form.name.trim()) {
    form.name = path.split(/[\\/]/).filter(Boolean).pop() ?? "";
  }
};

const submitForm = async () => {
  busy.value = true;
  form.error = "";
  try {
    if (form.id === 0) {
      await api.create(form.name, form.path);
    } else {
      await api.update(form.id, form.name, form.path);
    }
    form.open = false;
    await load();
  } catch (err: any) {
    form.error = err?.message || String(err);
  } finally {
    busy.value = false;
  }
};

const openDetect = async () => {
  detect.open = true;
  detect.loading = true;
  try {
    candidates.value = await api.detect();
  } catch (err: any) {
    $showError(err);
  } finally {
    detect.loading = false;
  }
};

const useCandidate = (candidate: IDiskCandidate) => {
  form.open = true;
  form.id = 0;
  form.name = candidate.name;
  form.path = candidate.path;
  form.error = "";
  detect.open = false;
};

const removeDisk = async (disk: IDisk) => {
  busy.value = true;
  try {
    await api.remove(disk.id);
    confirmDelete.value = null;
    await load();
  } catch (err: any) {
    $showError(err);
  } finally {
    busy.value = false;
  }
};
</script>
