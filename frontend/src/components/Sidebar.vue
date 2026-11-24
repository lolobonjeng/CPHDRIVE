<template>
  <div
    v-show="active"
    @click="closeHovers"
    class="overlay sidebar-scrim"
  ></div>
  <nav :class="{ active }">
    <template v-if="isLoggedIn">
      <div class="brand">
        <span>CPHDRIVE</span>
      </div>

      <button
        v-if="isFiles && user.perm.create"
        class="new-button"
        @click="openNew"
        aria-label="New"
        title="New"
      >
        <i class="material-icons">add</i>
        <span>{{ $t("buttons.new") }}</span>
      </button>

      <div class="nav-group">
        <button
          class="action"
          :class="{ 'action--active': isFiles }"
          @click="toRoot"
          :aria-label="$t('sidebar.myFiles')"
          :title="$t('sidebar.myFiles')"
        >
          <i class="material-icons">folder</i>
          <span>{{ $t("sidebar.myFiles") }}</span>
        </button>

        <button
          v-if="user.perm.admin"
          class="action"
          :class="{ 'action--active': $route.path === '/settings/global' }"
          @click="toGlobalSettings"
          :aria-label="$t('sidebar.settings')"
          :title="$t('sidebar.settings')"
        >
          <i class="material-icons">settings</i>
          <span>{{ $t("sidebar.settings") }}</span>
        </button>
      </div>

      <div class="nav-group">
        <button
          class="action"
          :class="{ 'action--active': $route.path === '/settings/profile' }"
          @click="toAccountSettings"
          :aria-label="user.username"
          :title="user.username"
        >
          <i class="material-icons">person</i>
          <span>{{ user.username }}</span>
        </button>

        <button
          class="action"
          @click="switchTheme"
          :aria-label="
            $t('settings.themes.' + (activeTheme === 'dark' ? 'light' : 'dark'))
          "
          :title="
            $t('settings.themes.' + (activeTheme === 'dark' ? 'light' : 'dark'))
          "
        >
          <i class="material-icons">{{
            activeTheme === "dark" ? "light_mode" : "dark_mode"
          }}</i>
          <span>{{
            $t("settings.themes." + (activeTheme === "dark" ? "light" : "dark"))
          }}</span>
        </button>

        <button
          class="action"
          @click="showHover('about')"
          aria-label="About"
          title="About"
        >
          <i class="material-icons">info_outline</i>
          <span>About</span>
        </button>

        <button
          v-if="canLogout"
          @click="logout"
          class="action"
          id="logout"
          :aria-label="$t('sidebar.logout')"
          :title="$t('sidebar.logout')"
        >
          <i class="material-icons">exit_to_app</i>
          <span>{{ $t("sidebar.logout") }}</span>
        </button>
      </div>
    </template>
    <template v-else>
      <router-link
        v-if="!hideLoginButton"
        class="action"
        to="/login"
        :aria-label="$t('sidebar.login')"
        :title="$t('sidebar.login')"
      >
        <i class="material-icons">exit_to_app</i>
        <span>{{ $t("sidebar.login") }}</span>
      </router-link>

      <router-link
        v-if="signup"
        class="action"
        to="/login"
        :aria-label="$t('sidebar.signup')"
        :title="$t('sidebar.signup')"
      >
        <i class="material-icons">person_add</i>
        <span>{{ $t("sidebar.signup") }}</span>
      </router-link>
    </template>

    <div class="nav-usage" v-if="isFiles && !disableUsedPercentage">
      <progress-bar :val="usage.usedPercentage" size="small"></progress-bar>
      <p>
        {{ $t("sidebar.diskUsed", { used: usage.used, total: usage.total }) }}
      </p>
    </div>

    <p class="credits">
      <span
        >CPHDRIVE<template
          v-if="version && !version.includes('untracked')"
        >
          ({{ version }})</template
        ></span
      >
      <span>
        <a @click="help">{{ $t("sidebar.help") }}</a>
      </span>
    </p>
  </nav>
</template>

<script>
import { reactive, ref } from "vue";
import { mapActions, mapState } from "pinia";
import { useAuthStore } from "@/stores/auth";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";
import { getTheme, toggleTheme } from "@/utils/theme";

import * as auth from "@/utils/auth";
import {
  version,
  signup,
  hideLoginButton,
  disableUsedPercentage,
  noAuth,
  logoutPage,
  loginPage,
} from "@/utils/constants";
import { files as api } from "@/api";
import ProgressBar from "@/components/ProgressBar.vue";
import prettyBytes from "pretty-bytes";

const USAGE_DEFAULT = { used: "0 B", total: "0 B", usedPercentage: 0 };

export default {
  name: "sidebar",
  setup() {
    const usage = reactive(USAGE_DEFAULT);
    const activeTheme = ref(getTheme());
    return {
      usage,
      activeTheme,
      usageAbortController: new AbortController(),
    };
  },
  components: {
    ProgressBar,
  },
  inject: ["$showError"],
  computed: {
    ...mapState(useAuthStore, ["user", "isLoggedIn"]),
    ...mapState(useFileStore, ["isFiles", "reload"]),
    ...mapState(useLayoutStore, ["currentPromptName"]),
    active() {
      return this.currentPromptName === "sidebar";
    },
    signup: () => signup,
    hideLoginButton: () => hideLoginButton,
    version: () => version,
    disableUsedPercentage: () => disableUsedPercentage,
    canLogout: () => !noAuth && (loginPage || logoutPage !== "/login"),
  },
  methods: {
    ...mapActions(useLayoutStore, ["closeHovers", "showHover"]),
    abortOngoingFetchUsage() {
      this.usageAbortController.abort();
    },
    async fetchUsage() {
      const path = this.$route.path.endsWith("/")
        ? this.$route.path
        : this.$route.path + "/";
      let usageStats = USAGE_DEFAULT;
      if (this.disableUsedPercentage) {
        return Object.assign(this.usage, usageStats);
      }
      try {
        this.abortOngoingFetchUsage();
        this.usageAbortController = new AbortController();
        const usage = await api.usage(path, this.usageAbortController.signal);
        usageStats = {
          used: prettyBytes(usage.used, { binary: true }),
          total: prettyBytes(usage.total, { binary: true }),
          usedPercentage: Math.round((usage.used / usage.total) * 100),
        };
      } finally {
        return Object.assign(this.usage, usageStats);
      }
    },
    toRoot() {
      this.$router.push({ path: "/files" });
      this.closeHovers();
    },
    // Drop the sidebar's own prompt before opening the menu. Otherwise closing
    // the menu pops back to "sidebar" and the drawer slides in again on phones.
    openNew() {
      this.closeHovers();
      this.showHover("new");
    },
    switchTheme() {
      toggleTheme();
      this.activeTheme = getTheme();
    },
    toAccountSettings() {
      this.$router.push({ path: "/settings/profile" });
      this.closeHovers();
    },
    toGlobalSettings() {
      this.$router.push({ path: "/settings/global" });
      this.closeHovers();
    },
    help() {
      this.showHover("help");
    },
    // The rail is permanent on a desktop, so a drawer left open from a phone
    // sized window is a leftover state. It matters because the scrim is a
    // full-screen backdrop-filter: left behind, it blurs and blocks the whole
    // app until you click it.
    dropDrawerOnDesktop() {
      if (window.innerWidth >= 737 && this.active) {
        this.closeHovers();
      }
    },
    logout: auth.logout,
  },
  watch: {
    $route: {
      handler(to) {
        if (to.path.includes("/files")) {
          this.fetchUsage();
        }
      },
      immediate: true,
    },
  },
  mounted() {
    window.addEventListener("resize", this.dropDrawerOnDesktop);
  },
  unmounted() {
    window.removeEventListener("resize", this.dropDrawerOnDesktop);
    this.abortOngoingFetchUsage();
  },
};
</script>
