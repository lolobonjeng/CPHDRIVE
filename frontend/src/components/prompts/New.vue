<template>
  <div class="card floating new-card">
    <div class="card-title">
      <h2>{{ t("buttons.new") }}</h2>
    </div>

    <div class="card-content">
      <button class="new-option" @click="open('newDir')">
        <i class="material-icons">create_new_folder</i>
        <span>
          <strong>{{ t("sidebar.newFolder") }}</strong>
          <small>Create an empty folder here</small>
        </span>
      </button>

      <button class="new-option" @click="open('newFile')">
        <i class="material-icons">note_add</i>
        <span>
          <strong>{{ t("sidebar.newFile") }}</strong>
          <small>Create an empty file here</small>
        </span>
      </button>

      <div class="new-divider"></div>

      <button class="new-option" @click="pick('upload-input')">
        <i class="material-icons">upload_file</i>
        <span>
          <strong>Upload file</strong>
          <small>Pick files from this device</small>
        </span>
      </button>

      <button class="new-option" @click="pick('upload-folder-input')">
        <i class="material-icons">drive_folder_upload</i>
        <span>
          <strong>Upload folder</strong>
          <small>Pick a folder from this device</small>
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/stores/layout";
import { useI18n } from "vue-i18n";

const layoutStore = useLayoutStore();
const { t } = useI18n();

// Replace this prompt rather than stacking on top of it, otherwise closing
// "new folder" drops the user back into this menu.
const open = (prompt: string) => {
  layoutStore.closeHovers();
  layoutStore.showHover(prompt);
};

// The hidden inputs live in the listing, so they only exist on a files route.
const pick = (id: string) => {
  layoutStore.closeHovers();
  document.getElementById(id)?.click();
};
</script>
