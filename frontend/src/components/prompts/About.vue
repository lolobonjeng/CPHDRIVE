<template>
  <div class="card floating about">
    <div class="card-title">
      <h2>About {{ appName }}</h2>
    </div>

    <div class="card-content">
      <p>
        {{ appName }} is your private drive. Every file lives on your own
        server, and you reach it from any device on your network.
      </p>

      <ul>
        <li v-if="version && !version.includes('untracked')">
          <strong>Version</strong>: {{ version }}
        </li>
        <li><strong>Hosting</strong>: self-hosted, no cloud account</li>
        <li>
          <strong>Included</strong>: permanent share links, HTML preview, theme
          switcher
        </li>
      </ul>

      <p>Open source, Apache 2.0.</p>
    </div>

    <div class="card-action">
      <button
        id="focus-prompt"
        type="submit"
        @click="closeHovers"
        class="button button--flat"
        :aria-label="$t('buttons.ok')"
        :title="$t('buttons.ok')"
        tabindex="1"
      >
        {{ $t("buttons.ok") }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/stores/layout";
import { name as appName, version } from "@/utils/constants";

const layoutStore = useLayoutStore();
const closeHovers = () => layoutStore.closeHovers();
</script>
