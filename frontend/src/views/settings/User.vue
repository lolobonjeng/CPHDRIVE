<template>
  <errors v-if="error" :errorCode="error.status" />
  <div class="row" v-else-if="!layoutStore.loading">
    <div class="column column--wide">
      <form @submit="save" class="card user-card">
        <div class="card-title">
          <h2 v-if="user?.id === 0">{{ $t("settings.newUser") }}</h2>
          <h2 v-else>{{ $t("settings.user") }} {{ user?.username }}</h2>
        </div>

        <div class="card-content" v-if="user">
          <user-form
            v-model:user="user"
            v-model:createUserDir="createUserDir"
            :isDefault="false"
            :isNew="isNew"
            :initialDiskID="originalDiskID"
            wide
          />
        </div>

        <div class="card-action">
          <div class="move-progress" v-if="moveState.running">
            <progress-bar :val="movePercent" size="small"></progress-bar>
            <p class="small">
              {{
                t("settings.moveProgress", {
                  done: prettyBytes(moveState.done, { binary: true }),
                  total: prettyBytes(moveState.total, { binary: true }),
                })
              }}
            </p>
          </div>
          <button
            v-if="!isNew"
            @click.prevent="deletePrompt"
            type="button"
            class="button button--flat button--red"
            :aria-label="$t('buttons.delete')"
            :title="$t('buttons.delete')"
          >
            {{ $t("buttons.delete") }}
          </button>
          <router-link to="/settings/users">
            <button
              class="button button--flat button--grey"
              :aria-label="$t('buttons.cancel')"
              :title="$t('buttons.cancel')"
            >
              {{ $t("buttons.cancel") }}
            </button>
          </router-link>
          <input
            class="button button--flat"
            type="submit"
            :value="$t('buttons.save')"
            :disabled="moveState.running"
          />
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useAuthStore } from "@/stores/auth";
import { useLayoutStore } from "@/stores/layout";
import { users as api, settings, disks as disksApi } from "@/api";
import UserForm from "@/components/settings/UserForm.vue";
import ProgressBar from "@/components/ProgressBar.vue";
import Errors from "@/views/Errors.vue";
import prettyBytes from "pretty-bytes";
import { computed, inject, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { StatusError } from "@/api/utils";
import { authMethod } from "@/utils/constants";
import { logout } from "@/utils/auth";

const error = ref<StatusError>();
const originalUser = ref<IUser>();
const user = ref<IUser>();
const createUserDir = ref<boolean>(false);
const isCurrentPasswordRequired = ref<boolean>(false);
const originalDiskID = ref<number>(0);
const moveState = ref({ running: false, done: 0, total: 0 });

const movePercent = computed(() => {
  if (moveState.value.total <= 0) return 0;
  return Math.min(
    100,
    Math.round((moveState.value.done / moveState.value.total) * 100)
  );
});

const $showError = inject<IToastError>("$showError")!;
const $showSuccess = inject<IToastSuccess>("$showSuccess")!;

const authStore = useAuthStore();
const layoutStore = useLayoutStore();
const route = useRoute();
const router = useRouter();
const { t } = useI18n();

onMounted(() => {
  fetchData();
});

const isNew = computed(() => route.path === "/settings/users/new");

watch(route, () => fetchData());
watch(user, () => {
  if (!user.value?.perm.admin) return;
  user.value.lockPassword = false;
});

const fetchData = async () => {
  layoutStore.loading = true;

  try {
    if (isNew.value) {
      const { defaults, createUserDir: _createUserDir } = await settings.get();
      isCurrentPasswordRequired.value = authMethod == "json";
      createUserDir.value = _createUserDir;
      user.value = {
        ...defaults,
        username: "",
        password: "",
        rules: [],
        lockPassword: false,
        id: 0,
      };
      originalDiskID.value = user.value.diskID ?? 0;
    } else {
      const { authMethod } = await settings.get();
      isCurrentPasswordRequired.value = authMethod == "json";
      const id = Array.isArray(route.params.id)
        ? route.params.id.join("")
        : route.params.id;
      const loaded = await api.get(parseInt(id));
      user.value = { ...loaded };
      originalDiskID.value = loaded.diskID ?? 0;
    }
  } catch (err) {
    if (err instanceof Error) {
      error.value = err;
    }
  } finally {
    layoutStore.loading = false;
  }
};

const deletePrompt = () => {
  if (isCurrentPasswordRequired.value) {
    layoutStore.showHover({
      prompt: "current-password",
      confirm: (event: Event, currentPassword: string) => {
        event.preventDefault();
        layoutStore.closeHovers();
        deleteUser(currentPassword);
      },
    });
  } else {
    layoutStore.showHover({
      prompt: "deleteUser",
      confirm: () => deleteUser(""),
    });
  }
};

const deleteUser = async (currentPassword: string) => {
  if (!user.value) {
    return false;
  }
  try {
    await api.remove(user.value.id, currentPassword);
    if (user.value.id == authStore.user?.id) {
      logout();
    } else {
      router.push({ path: "/settings/users" });
    }
    $showSuccess(t("settings.userDeleted"));
  } catch (err) {
    if (err instanceof StatusError) {
      err.status === 403 ? $showError(t("errors.forbidden")) : $showError(err);
    } else if (err instanceof Error) {
      $showError(err);
    }
  }

  return true;
};

const save = (event: Event) => {
  event.preventDefault();
  if (isCurrentPasswordRequired.value) {
    layoutStore.showHover({
      prompt: "current-password",
      confirm: (event: Event, currentPassword: string) => {
        event.preventDefault();
        layoutStore.closeHovers();
        send(currentPassword);
      },
    });
  } else {
    send("");
  }

  return true;
};

const waitForMove = async (userID: number, diskID: number) => {
  const { id } = await disksApi.moveUser(userID, diskID);
  moveState.value = { running: true, done: 0, total: 0 };
  $showSuccess(t("settings.moveStarted"));

  const started = Date.now();
  const limit = 24 * 60 * 60 * 1000;

  while (Date.now() - started < limit) {
    await new Promise((resolve) => setTimeout(resolve, 800));
    const job = await disksApi.getJob(id);
    moveState.value = {
      running: job.state === "running",
      done: job.copied,
      total: job.total,
    };

    if (job.state === "done") {
      $showSuccess(t("settings.moveDone"));
      return;
    }
    if (job.state === "failed") {
      throw new Error(job.error || t("settings.moveFailed"));
    }
  }

  throw new Error(t("settings.moveFailed"));
};

const send = async (currentPassword: string) => {
  if (!user.value) {
    return false;
  }

  try {
    if (isNew.value) {
      const newUser: IUser = {
        ...originalUser?.value,
        ...user.value,
      };

      const loc = await api.create(newUser, currentPassword);
      router.push({ path: loc || "/settings/users" });
      $showSuccess(t("settings.userCreated"));
    } else {
      // The disk itself is changed by the move endpoint, which copies the
      // files first. Send the old ID here so the plain update does not get
      // rejected, then run the move.
      const targetDisk = user.value.diskID ?? originalDiskID.value;
      const payload = { ...user.value, diskID: originalDiskID.value };

      await api.update(payload, ["all"], currentPassword);

      if (user.value.id === authStore.user?.id) {
        authStore.updateUser(payload);
      }

      if (targetDisk !== originalDiskID.value) {
        await waitForMove(user.value.id, targetDisk);
        originalDiskID.value = targetDisk;
        user.value.diskID = targetDisk;
        if (user.value.id === authStore.user?.id) {
          authStore.updateUser(user.value);
        }
      }

      $showSuccess(t("settings.userUpdated"));
    }
  } catch (e: any) {
    moveState.value.running = false;
    $showError(e);
  }
};
</script>
