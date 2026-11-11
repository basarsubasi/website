<script setup lang="ts">
import { computed } from "vue";
import { formatPostDate, postParagraphs } from "./post-layout";
const props = defineProps<{ title: string; body: string; date: string }>();
const paragraphs = computed(() => postParagraphs(props.body));
</script>
<template>
  <article class="post-article">
    <div class="post-heading">
      <time :datetime="date">{{ formatPostDate(date) }}</time>
      <h1 class="post-title"><slot name="title">{{ title }}</slot></h1>
    </div>
    <slot name="body">
      <div class="post-body">
        <p v-for="(paragraph, index) in paragraphs" :key="index">{{ paragraph }}</p>
      </div>
    </slot>
  </article>
</template>
