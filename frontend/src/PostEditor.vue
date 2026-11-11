<script setup lang="ts">
import { reactive } from "vue";
import type { Post, PostInput } from "./types";
const props = defineProps<{ post: Post | null; busy: boolean }>();
const emit = defineEmits<{ save: [value: PostInput]; cancel: [] }>();
const form = reactive<PostInput>({
  slug: props.post?.slug ?? "",
  title: props.post?.title ?? "",
  excerpt: props.post?.excerpt ?? "",
  body: props.post?.body ?? "",
  published: props.post?.published ?? 0,
});
function suggestSlug() {
  if (!form.slug)
    form.slug = form.title
      .toLowerCase()
      .normalize("NFKD")
      .replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "");
}
</script>
<template>
  <form class="editor" @submit.prevent="emit('save', { ...form })">
    <h2>{{ post ? "Edit post" : "New post" }}</h2>
    <fieldset :disabled="busy">
      <label
        >Title<input
          v-model="form.title"
          required
          maxlength="300"
          @blur="suggestSlug" /></label
      ><label
        >Slug<input
          v-model="form.slug"
          required
          pattern="[a-z0-9]+(-[a-z0-9]+)*"
          maxlength="200"
          placeholder="my-first-post" /></label
      ><label
        >Excerpt<textarea v-model="form.excerpt" rows="2"></textarea></label
      ><label
        >Body<textarea v-model="form.body" required rows="18"></textarea>
      </label>
      <p class="hint">Plain text. Paragraph breaks are preserved.</p>
      <label
        >Status<select v-model="form.published">
          <option :value="0">Draft — visible only in admin</option>
          <option :value="1">Published — visible to everyone</option>
        </select></label
      >
      <div class="actions">
        <button type="submit">{{ busy ? "Saving…" : "Save post" }}</button
        ><button type="button" @click="emit('cancel')">Cancel</button>
      </div>
    </fieldset>
  </form>
</template>
