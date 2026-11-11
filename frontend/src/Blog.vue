<script setup lang="ts">
import { ref, watch, computed } from "vue";
import { useRoute } from "vue-router";
import { api, message } from "./api";
import PostArticle from "./PostArticle.vue";
import { formatPostDate } from "./post-layout";
import type { Post, ListResponse, ItemResponse } from "./types";
const route = useRoute(),
  posts = ref<Post[]>([]),
  post = ref<Post | null>(null),
  error = ref(""),
  loading = ref(false),
  total = ref(0);
const page = computed(() =>
  Math.max(1, Number.parseInt(String(route.query.page)) || 1),
);
let request = 0;
watch(
  () => route.fullPath,
  async () => {
    const id = ++request;
    loading.value = true;
    error.value = "";
    post.value = null;
    posts.value = [];
    try {
      if (route.params.slug) {
        const result = await api<ItemResponse<Post>>(
          "/posts/" + encodeURIComponent(String(route.params.slug)),
        );
        if (id === request) post.value = result.data;
      } else {
        const result = await api<ListResponse<Post>>(
          `/posts?page=${page.value}`,
        );
        if (id === request) {
          posts.value = result.data;
          total.value = result.total;
        }
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
  <section class="blog" :aria-busy="loading">
    <p v-if="error" role="alert">{{ error }}</p>
    <PostArticle v-else-if="!loading && post" :title="post.title" :body="post.body" :date="post.created_at" />
    <template v-else-if="!loading">
      <p v-if="!posts.length">Henüz yazı yok.</p>
      <article v-for="item in posts" :key="item.id" class="post-summary">
        <time :datetime="item.created_at">{{
          formatPostDate(item.created_at)
        }}</time>
        <h2>
          <RouterLink :to="'/blog/' + item.slug">{{ item.title }}</RouterLink>
        </h2>
        <p>{{ item.excerpt }}</p>
      </article>
      <div v-if="total > 10" class="pagination">
        <button
          :disabled="page === 1"
          @click="$router.push({ query: { page: page - 1 } })"
        >
          Önceki</button
        ><span>{{ page }}</span
        ><button
          :disabled="page * 10 >= total"
          @click="$router.push({ query: { page: page + 1 } })"
        >
          Sonraki
        </button>
      </div></template
    >
  </section>
</template>
