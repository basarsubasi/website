<script setup lang="ts">
import { ref, watch } from "vue";
import { useRoute } from "vue-router";
import { api, message } from "./api";
import type { Photo, ItemResponse } from "./types";
const route = useRoute(),
  image = ref<Photo | null>(null),
  error = ref("");
const imageLoaded = ref(false);
let request = 0;
watch(
  () => route.params.uuid,
  async (uuid) => {
    const id = ++request;
    image.value = null;
    imageLoaded.value = false;
    error.value = "";
    try {
      const data = await api<ItemResponse<Photo>>(
        "/images/" + encodeURIComponent(String(uuid)),
      );
      if (id === request) image.value = data.data;
    } catch (e) {
      if (id === request) error.value = message(e);
    }
  },
  { immediate: true },
);
</script>
<template>
  <p v-if="error" class="notice" role="alert">{{ error }}</p>
  <div v-else-if="image" class="single-image-page">
    <img class="public-photo" :class="{ 'is-loaded': imageLoaded }" @load="imageLoaded = true" :src="image.image_url" :alt="image.name || 'Fotoğraf'" :width="image.width || undefined" :height="image.height || undefined" loading="eager" fetchpriority="high" decoding="async" />
  </div>
</template>
