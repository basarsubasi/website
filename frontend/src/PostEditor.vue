<script setup lang="ts">
import { nextTick, onMounted, reactive, ref, watch } from "vue";
import { EditorContent, useEditor } from "@tiptap/vue-3";
import type { JSONContent } from "@tiptap/core";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import HardBreak from "@tiptap/extension-hard-break";
import { Placeholder, UndoRedo } from "@tiptap/extensions";
import PostArticle from "./PostArticle.vue";
import { postParagraphs } from "./post-layout";
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

function paragraphsToDocument(body: string): JSONContent[] {
  const paragraphs = postParagraphs(body);
  return (paragraphs.length ? paragraphs : [""]).map((paragraph) => ({
    type: "paragraph",
    content: paragraph.split("\n").flatMap((line, index): JSONContent[] => [
      ...(index ? [{ type: "hardBreak" }] : []),
      ...(line ? [{ type: "text", text: line }] : []),
    ]),
  }));
}
const editor = useEditor({
  extensions: [Document, Paragraph, Text, HardBreak, UndoRedo, Placeholder.configure({ placeholder: "Yazmaya başlayın…" })],
  content: { type: "doc", content: paragraphsToDocument(form.body) },
  editable: !props.busy,
  editorProps: {
    attributes: { role: "textbox", "aria-label": "İçerik", "aria-multiline": "true", "aria-required": "true", spellcheck: "true" },
    handlePaste(_view, event) {
      const text = event.clipboardData?.getData("text/plain");
      if (!text) return false;
      editor.value?.commands.insertContent(paragraphsToDocument(text));
      return true;
    },
  },
  onUpdate({ editor }) {
    form.body = editor.getText({ blockSeparator: "\n\n" });
    error.value = "";
  },
});
watch(() => props.busy, (busy) => editor.value?.setEditable(!busy));
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
  if (props.busy) return;
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
  if (!form.body.trim()) {
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
      <span class="hint">{{ post ? "Yazıyı düzenle" : "Yeni yazı" }}</span>
      <div class="actions">
        <button type="button" :disabled="busy" :aria-expanded="settingsOpen" aria-controls="post-settings" @click="settingsOpen = !settingsOpen">Yazı ayarları</button>
        <button type="submit" :disabled="busy">Yazıyı kaydet</button>
        <button type="button" :disabled="busy" @click="emit('cancel')">Vazgeç</button>
      </div>
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <fieldset v-show="settingsOpen" id="post-settings" class="post-settings" :disabled="busy">
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
