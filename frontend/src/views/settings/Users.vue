<template>
  <errors v-if="error" :errorCode="error.status" />
  <div class="row" v-else-if="!layoutStore.loading">
    <div class="column column--wide">
      <div class="card">
        <div class="card-title">
          <h2>{{ t("settings.users") }}</h2>
          <router-link to="/settings/users/new"
            ><button class="button">
              {{ t("buttons.new") }}
            </button></router-link
          >
        </div>

        <div class="card-content full user-table-wrap">
          <table>
            <tr>
              <th>{{ t("settings.username") }}</th>
              <th>{{ t("settings.admin") }}</th>
              <th>{{ t("settings.scope") }}</th>
              <th>{{ t("settings.disk") }}</th>
              <th>{{ t("settings.quota") }}</th>
              <th>{{ t("settings.used") }}</th>
              <th></th>
            </tr>

            <tr v-for="user in users" :key="user.id">
              <td>{{ user.username }}</td>
              <td>
                <i v-if="user.perm.admin" class="material-icons">done</i
                ><i v-else class="material-icons">close</i>
              </td>
              <td>{{ user.scope }}</td>
              <td>
                <span v-if="user.diskOffline" class="user-offline">
                  {{ t("settings.offline") }}
                </span>
                <template v-else>
                  {{ diskNames[user.diskID] || t("settings.systemDisk") }}
                </template>
              </td>
              <td>
                {{
                  user.quota
                    ? prettyBytes(user.quota, { binary: true })
                    : t("settings.unlimited")
                }}
              </td>
              <td>
                <span
                  v-if="usages[user.id] && !usages[user.id].offline"
                  class="user-usage"
                >
                  {{ prettyBytes(usages[user.id].used, { binary: true }) }}
                  <span class="user-usage-bar" v-if="user.quota">
                    <span :style="{ width: usagePercent(user) + '%' }"></span>
                  </span>
                </span>
                <span v-else>—</span>
              </td>
              <td class="small">
                <router-link :to="'/settings/users/' + user.id"
                  ><i class="material-icons">mode_edit</i></router-link
                >
              </td>
            </tr>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/stores/layout";
import { users as api, disks as disksApi } from "@/api";
import Errors from "@/views/Errors.vue";
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { StatusError } from "@/api/utils";
import prettyBytes from "pretty-bytes";

const error = ref<StatusError | null>(null);
const users = ref<IUser[]>([]);
const usages = ref<Record<number, IUserUsage>>({});
const diskNames = ref<Record<number, string>>({});

const layoutStore = useLayoutStore();
const { t } = useI18n();

const usagePercent = (user: IUser) => {
  const usage = usages.value[user.id];
  if (!usage || !user.quota) return 0;
  return Math.min(100, Math.round((usage.used / user.quota) * 100));
};

onMounted(async () => {
  layoutStore.loading = true;

  try {
    const [list, diskList] = await Promise.all([
      api.getAll(),
      disksApi.getAll(),
    ]);
    users.value = list;
    diskNames.value = Object.fromEntries(
      diskList.map((disk) => [disk.id, disk.name])
    );

    try {
      const usageList = await api.getUsage();
      usages.value = Object.fromEntries(usageList.map((u) => [u.id, u]));
    } catch (e) {
      // The quota column still works; the live usage column stays empty.
    }
  } catch (err) {
    if (err instanceof Error) {
      error.value = err;
    }
  } finally {
    layoutStore.loading = false;
  }
});
</script>
