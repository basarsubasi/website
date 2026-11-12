<script setup lang="ts">
import { nextTick, onMounted, reactive, ref, watch } from "vue";
import { EditorContent, useEditor } from "@tiptap/vue-3";
import { NodeSelection } from "@tiptap/pm/state";
import { BlogImage } from "./blog-image";
import { paragraphsToDocument, readPostBody, writePostBody } from "./post-content";
import { api, message } from "./api";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import HardBreak from "@tiptap/extension-hard-break";
import { Placeholder, UndoRedo } from "@tiptap/extensions";
import PostArticle from "./PostArticle.vue";

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
const initialTitle = form.title;
const date = props.post?.created_at ?? new Date().toISOString();
const settingsOpen = ref(false);
const error = ref("");
const titleField = ref<HTMLElement>();
const formElement = ref<HTMLFormElement>();

const uploading = ref(false);
const imageInput = ref<HTMLInputElement>();
async function uploadImage(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file || !editor.value || props.busy || uploading.value) return;
  if (file.size > 25 * 1024 * 1024) { error.value = "Görsel en fazla 25 MiB olabilir."; input.value = ""; return; }
  const position = editor.value.state.selection.from;
  uploading.value = true;
  editor.value.setEditable(false);
  error.value = "";
  try {
    const data = new FormData();
    data.append("image", file);
    const result = await api<{ data: { url: string; width: number; height: number } }>("/admin/blog/images", { method: "POST", body: data });
    editor.value.chain().focus().insertContentAt(position, [{ type: "blogImage", attrs: { src: result.data.url, width: 100, intrinsicWidth: result.data.width, intrinsicHeight: result.data.height } }, { type: "paragraph" }]).run();
  } catch (err) { error.value = message(err); }
  finally { uploading.value = false; editor.value?.setEditable(!props.busy); input.value = ""; }
}
const editor = useEditor({
  extensions: [BlogImage, Document, Paragraph, Text, HardBreak, UndoRedo, Placeholder.configure({ placeholder: "Yazmaya başlayın…" })],
  content: { type: "doc", content: readPostBody(form.body) },
  editable: !props.busy,
  editorProps: {
    attributes: { role: "textbox", "aria-label": "İçerik", "aria-multiline": "true", "aria-required": "true", spellcheck: "true" },
    handleClickOn(view, _pos, node, nodePos, event) {
      if (node.type.name !== "blogImage" || !(event.target instanceof HTMLImageElement)) return false;
      view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, nodePos)));
      return true;
    },
    handlePaste(_view, event) {
      const text = event.clipboardData?.getData("text/plain");
      if (!text) return false;
      if (editor.value?.isActive("blogImage")) return false;
      editor.value?.commands.insertContent(paragraphsToDocument(text));
      return true;
    },
  },
  onUpdate({ editor }) {
    form.body = writePostBody(editor.getJSON().content ?? []);
    error.value = "";
  },
});
watch(() => props.busy, (busy) => editor.value?.setEditable(!busy && !uploading.value));
onMounted(() => { if (!props.post) titleField.value?.focus(); });

function updateTitle(event: Event) {
  form.title = (event.currentTarget as HTMLElement).innerText.replace(/\r?\n/g, " ");
  error.value = "";
}
function suggestSlug() {
  if (!form.slug)
    form.slug = form.title
      .toLowerCase()
      .normalize("NFKD")
      .replace(/ı/g, "i")
      .replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "");
}
async function save() {
  if (props.busy || uploading.value) return;
  if (!form.title.trim()) {
    error.value = "Bir başlık yazın.";
    titleField.value?.focus();
    return;
  }
  if (new TextEncoder().encode(form.title.trim()).length > 300) {
    error.value = "Başlık çok uzun. Lütfen kısaltın.";
    titleField.value?.focus();
    return;
  }
  if (!editor.value?.getText().trim() && !editor.value?.getJSON().content?.some(node => node.type === "blogImage")) {
    error.value = "Yazınızın içeriğini ekleyin.";
    editor.value?.commands.focus();
    return;
  }
  suggestSlug();
  if (!formElement.value?.checkValidity()) {
    settingsOpen.value = true;
    await nextTick();
    formElement.value?.reportValidity();
    return;
  }
  emit("save", { ...form });
}
</script>
<template>
  <form ref="formElement" class="post-editor" novalidate @submit.prevent="save">
    <div class="post-editor-toolbar">
      <input ref="imageInput" type="file" accept="image/jpeg,image/png,image/gif" hidden @change="uploadImage" />
      <span class="hint">{{ post ? "Yazıyı düzenle" : "Yeni yazı" }}</span>
      <div class="actions">
        <button type="button" :disabled="busy || uploading" @click="imageInput?.click()">{{ uploading ? "Yükleniyor…" : "Görsel ekle" }}</button>
        <button type="button" :disabled="busy || uploading" :aria-expanded="settingsOpen" aria-controls="post-settings" @click="settingsOpen = !settingsOpen">Yazı ayarları</button>
        <button type="submit" :disabled="busy || uploading">Yazıyı kaydet</button>
        <button type="button" :disabled="busy || uploading" @click="emit('cancel')">Vazgeç</button>
      </div>
    </div>
    <div v-if="editor?.isActive('blogImage')" class="image-size-controls" aria-label="Görsel boyutu">
      <span>Görsel genişliği</span>
      <button v-for="width in [25, 50, 75, 100]" :key="width" type="button" :disabled="busy || uploading" :aria-pressed="editor.getAttributes('blogImage').width === width" @click="editor.chain().focus().updateAttributes('blogImage', { width }).run()">%{{ width }}</button>
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <fieldset v-show="settingsOpen" id="post-settings" class="post-settings" :disabled="busy || uploading">
      <label>Kısa bağlantı<input v-model="form.slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="200" placeholder="ilk-yazim" /></label>
      <label>Özet<textarea v-model="form.excerpt" rows="2" maxlength="2000"></textarea></label>
      <label>Durum<select v-model="form.published">
        <option :value="0">Taslak — yalnızca yönetim panelinde görünür</option>
        <option :value="1">Yayında — herkese açık</option>
      </select></label>
    </fieldset>
    <PostArticle :title="form.title" :body="form.body" :date="date">
      <template #title>
        <span ref="titleField" class="editable-title" :contenteditable="busy ? 'false' : 'plaintext-only'" role="textbox" aria-label="Başlık" aria-required="true" aria-multiline="false" :aria-disabled="busy" data-placeholder="Yazının başlığı" spellcheck="true" @input="updateTitle" @blur="suggestSlug" @keydown.enter.prevent="editor?.commands.focus()">{{ initialTitle }}</span>
      </template>
      <template #body><EditorContent :editor="editor" class="post-body" /></template>
    </PostArticle>
  </form>
</template>
