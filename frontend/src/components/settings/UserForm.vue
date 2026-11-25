<template>
  <div>
    <p v-if="!isDefault && props.user !== null">
      <label for="username">{{ t("settings.username") }}</label>
      <input
        class="input input--block"
        type="text"
        v-model="user.username"
        id="username"
      />
    </p>

    <p v-if="!isDefault">
      <label for="password">{{ t("settings.password") }}</label>
      <input
        class="input input--block"
        type="password"
        :placeholder="passwordPlaceholder"
        v-model="user.password"
        id="password"
      />
    </p>

    <p>
      <label for="scope">{{ t("settings.scope") }}</label>
      <input
        :disabled="createUserDirData ?? false"
        :placeholder="scopePlaceholder"
        class="input input--block"
        type="text"
        v-model="user.scope"
        id="scope"
      />
    </p>
    <p class="small" v-if="displayHomeDirectoryCheckbox">
      <input type="checkbox" v-model="createUserDirData" />
      {{ t("settings.createUserHomeDirectory") }}
    </p>

    <p>
      <label for="locale">{{ t("settings.language") }}</label>
      <languages
        class="input input--block"
        id="locale"
        v-model:locale="user.locale"
      ></languages>
    </p>

    <p v-if="!isDefault && user.perm">
      <input
        type="checkbox"
        :disabled="user.perm.admin"
        v-model="user.lockPassword"
      />
      {{ t("settings.lockPassword") }}
    </p>

    <permissions v-model:perm="user.perm" />
    <commands v-if="enableExec" v-model:commands="user.commands" />

    <div v-if="!isDefault" class="user-storage">
      <h3>{{ t("settings.storage") }}</h3>
      <p>
        <label for="disk">{{ t("settings.disk") }}</label>
        <select class="input input--block" id="disk" v-model.number="user.diskID">
          <option v-for="disk in disks" :key="disk.id" :value="disk.id">
            {{ disk.name }} — {{ disk.path }}
          </option>
        </select>
      </p>
      <p>
        <label for="quota">{{ t("settings.quotaLabel") }}</label>
        <input
          class="input input--block"
          id="quota"
          type="number"
          min="0"
          step="any"
          v-model.number="quotaGB"
        />
        <span class="small">{{ t("settings.quotaHelp") }}</span>
      </p>
      <p
        class="small user-storage-warning"
        v-if="!isNew && user.diskID !== (initialDiskID ?? 0)"
      >
        {{ t("settings.diskChangeWarning") }}
      </p>
    </div>

    <div v-if="!isDefault">
      <h3>{{ t("settings.rules") }}</h3>
      <p class="small">{{ t("settings.rulesHelp") }}</p>
      <rules v-model:rules="user.rules" />
    </div>
  </div>
</template>

<script setup lang="ts">
import Languages from "./Languages.vue";
import Rules from "./Rules.vue";
import Permissions from "./Permissions.vue";
import Commands from "./Commands.vue";
import { disks as disksApi } from "@/api";
import { enableExec } from "@/utils/constants";
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

const createUserDirData = ref<boolean | null>(null);
const originalUserScope = ref<string | null>(null);
const disks = ref<IDisk[]>([]);

const props = defineProps<{
  user: IUserForm;
  isNew: boolean;
  isDefault: boolean;
  createUserDir?: boolean;
  // The disk the user was loaded on. The parent owns it because a successful
  // move changes it, and the warning must disappear when it does.
  initialDiskID?: number;
}>();

onMounted(async () => {
  if (props.user.scope) {
    originalUserScope.value = props.user.scope;
    createUserDirData.value = props.createUserDir;
  }

  if (props.isDefault) {
    return;
  }

  try {
    disks.value = await disksApi.getAll();
  } catch (e) {
    // The form still works; the select just stays empty.
  }
});

// The quota is stored in bytes but edited in GiB. Keeping the edited number
// in its own ref means a value like 0.002 does not get rounded back to 0 and
// silently read as unlimited while the admin is still typing.
const quotaGB = ref<number>(0);

const fromBytes = (bytes: number) => {
  if (!bytes) return 0;
  return Math.round((bytes / (1 << 30)) * 1e6) / 1e6;
};

const toBytes = (gb: number) =>
  Number.isFinite(gb) && gb > 0 ? Math.round(gb * (1 << 30)) : 0;

watch(
  () => props.user.quota,
  (quota) => {
    const bytes = quota ?? 0;
    if (Math.abs(bytes - toBytes(quotaGB.value)) > 1) {
      quotaGB.value = fromBytes(bytes);
    }
  },
  { immediate: true }
);

watch(quotaGB, (gb) => {
  props.user.quota = toBytes(Number(gb));
});

const passwordPlaceholder = computed(() =>
  props.isNew ? "" : t("settings.avoidChanges")
);
const scopePlaceholder = computed(() =>
  createUserDirData.value ? t("settings.userScopeGenerationPlaceholder") : ""
);
const displayHomeDirectoryCheckbox = computed(
  () => props.isNew && createUserDirData.value
);

watch(
  () => props.user,
  () => {
    if (!props.user?.perm?.admin) return;
    props.user.lockPassword = false;
  }
);

watch(createUserDirData, () => {
  if (props.user?.scope) {
    props.user.scope = createUserDirData.value
      ? ""
      : (originalUserScope.value ?? "");
  }
});
</script>
