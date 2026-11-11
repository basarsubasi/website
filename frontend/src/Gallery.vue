<script setup lang="ts">
import { ref, watch, computed } from "vue";
import { useRoute } from "vue-router";
import { api, message } from "./api";
import type { Photo, ListResponse } from "./types";
const route = useRoute(),
  images = ref<Photo[]>([]),
  total = ref(0),
  loading = ref(false),
  error = ref("");
let request = 0;
const page = computed(() =>
  Math.max(1, Number.parseInt(String(route.query.page)) || 1),
);
watch(
  () => route.fullPath,
  async () => {
    const id = ++request;
    loading.value = true;
    error.value = "";
    images.value = [];
    const color =
      route.path === "/bw-photos"
        ? "bw"
        : route.path === "/color-photos"
          ? "color"
          : "";
    try {
      const data = await api<ListResponse<Photo>>(
        `/images?page=${page.value}&limit=9&color=${color}`,
      );
      if (id === request) {
        images.value = data.data;
        total.value = data.total;
      }
    } catch (e) {
      if (id === request) error.value = message(e);
    } finally {
      if (id === request) loading.value = false;
    }
  },
  { immediate: true },
);
</script>
<template>
  <div class="homepage">
    <p v-if="error" class="notice" role="alert">{{ error }}</p>
    <p v-else-if="loading" class="notice" role="status">Loading photos…</p>
    <template v-else>
      <div class="grid">
        <RouterLink
          v-for="image in images"
          :key="image.uuid"
          :to="`/image/${image.uuid}`"
          class="grid-item"
          ><img
            :src="image.image_url"
            :alt="image.name || 'photo'"
            loading="eager"
        /></RouterLink>
      </div>
      <p v-if="!images.length" class="notice">No photos yet.</p>
      <div class="pagination">
        <button
          :disabled="page === 1"
          aria-label="Previous page"
          @click="$router.push({ query: { page: page - 1 } })"
        >
          &lt;</button
        ><span>{{ page }}</span
        ><button
          :disabled="page >= Math.ceil(total / 9)"
          aria-label="Next page"
          @click="$router.push({ query: { page: page + 1 } })"
        >
          &gt;
        </button>
      </div>
    </template>
  </div>
</template>
