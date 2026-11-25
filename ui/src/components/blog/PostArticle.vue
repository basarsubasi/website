<script setup lang="ts">
import { computed } from "vue";
import { formatPostDate } from "./post-layout";
import { readPostBody, blockText } from "./post-content";
const props = defineProps<{ title: string; body: string; date: string }>();
const paragraphs = computed(() => readPostBody(props.body));
</script>
<template>
  <article class="post-article">
    <div class="post-heading">
      <time :datetime="date">{{ formatPostDate(date) }}</time>
      <h1 class="post-title"><slot name="title">{{ title }}</slot></h1>
    </div>
    <slot name="body">
      <div class="post-body">
        <template v-for="(block, index) in paragraphs" :key="index">
          <figure v-if="block.type === 'blogImage'" class="blog-image" >
            <img :style="{ width: block.attrs?.width + '%' }" :src="block.attrs?.src" :width="block.attrs?.intrinsicWidth || undefined" :height="block.attrs?.intrinsicHeight || undefined" alt="" :loading="index < 2 ? 'eager' : 'lazy'" :fetchpriority="index < 2 ? 'high' : 'auto'" decoding="async" />
            <figcaption v-if="blockText(block)">{{ blockText(block) }}</figcaption>
          </figure>
          <p v-else>{{ blockText(block) }}</p>
        </template>
      </div>
    </slot>
  </article>
</template>

<style>
.post-body .blog-image { margin: 1.5rem auto; width: 100%; max-width: 100%; }
.post-body .blog-image img { display: block; width: 100%; height: auto; margin: 0 auto; }
.post-body .blog-image figcaption { padding: .5rem .35rem .25rem; color: inherit; text-align: center; font-size: .8em; font-style: italic; line-height: 1.5; white-space: pre-wrap; }
.post-body .tiptap .blog-image { cursor: pointer; }
.post-body .tiptap .blog-image:focus-within, .post-body .blog-image.ProseMirror-selectednode { outline: 1px solid #888; outline-offset: 5px; }
.post-body .tiptap figcaption:empty::before, .post-body .tiptap figcaption:has(> br:only-child)::before { content: attr(data-placeholder); color: #999; pointer-events: none; }
.blog-image-frame { position: relative; margin: 0 auto; }
.image-size-controls { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; margin: 1rem 0; }
.image-size-controls .image-delete-control { color: #a11; }
.image-size-controls button[aria-pressed="true"] { background: #ddd; color: #222; }
</style>
