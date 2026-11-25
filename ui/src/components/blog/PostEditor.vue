<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from "vue";
import { EditorContent, useEditor } from "@tiptap/vue-3";
import { NodeSelection } from "@tiptap/pm/state";
import { BlogAudio, BlogImage, BlogVideo } from "./blog-media";
import { isMediaNode, mediaNodeTypes, type MediaNodeType, paragraphsToDocument, readPostBody, writePostBody } from "./post-content";
import { api, message, send } from "../../services/api";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import HardBreak from "@tiptap/extension-hard-break";
import { Placeholder, UndoRedo } from "@tiptap/extensions";
import PostArticle from "./PostArticle.vue";

import type { Post, PostInput } from "../../services/types";

const props = defineProps<{ post: Post | null; busy: boolean }>();
const emit = defineEmits<{ save: [value: PostInput]; cancel: []; dirty: [value: boolean] }>();
const form = reactive<PostInput>({
  slug: props.post?.slug ?? "",
  title: props.post?.title ?? "",
  excerpt: props.post?.excerpt ?? "",
  body: props.post?.body ?? "",
  published: props.post?.published ?? 0,
});
const snapshot = () => JSON.stringify({ ...form, body: readPostBody(form.body) });
const initialForm = snapshot();
const date = props.post?.created_at ?? new Date().toISOString();
const settingsOpen = ref(false);
const error = ref("");
const titleField = ref<HTMLTextAreaElement>();
const formElement = ref<HTMLFormElement>();

const uploading = ref<MediaNodeType | null>(null);
const deletingMedia = ref(false);
watch(() => !!uploading.value || deletingMedia.value || snapshot() !== initialForm,
  value => emit("dirty", value), { immediate: true, flush: "sync" });
const deletedMedia = new Set<string>();
const imageInput = ref<HTMLInputElement>();
const videoInput = ref<HTMLInputElement>();
const audioInput = ref<HTMLInputElement>();
const mediaSettings = {
  blogImage: {
    field: "image", folder: "images", width: "Görsel genişliği", remove: "Görseli sil",
    deleteTitle: "Görseli kalıcı olarak sil", sizeLabel: "Görsel", maxMiB: 25,
  },
  blogAudio: {
    field: "audio", folder: "audio", width: "Ses oynatıcı genişliği", remove: "Sesi sil",
    deleteTitle: "Sesi kalıcı olarak sil", sizeLabel: "Ses dosyası", maxMiB: 25,
  },
  blogVideo: {
    field: "video", folder: "videos", width: "Video genişliği", remove: "Videoyu sil",
    deleteTitle: "Videoyu kalıcı olarak sil", sizeLabel: "Video", maxMiB: 100,
  },
} as const;
async function uploadMedia(event: Event, type: MediaNodeType) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file || !editor.value || props.busy || uploading.value || deletingMedia.value) return;
  const settings = mediaSettings[type];
  if (file.size > settings.maxMiB * 1024 * 1024) {
    error.value = `${settings.sizeLabel} en fazla ${settings.maxMiB} MiB olabilir.`;
    input.value = "";
    return;
  }
  const position = editor.value.state.selection.from;
  uploading.value = type;
  editor.value.setEditable(false, false);
  error.value = "";
  try {
    const data = new FormData();
    data.append(settings.field, file);
    const result = await api<{ data: { url: string; width?: number; height?: number } }>(`/admin/blog/${settings.folder}`, { method: "POST", body: data });
    editor.value.chain().focus().insertContentAt(position, [{ type, attrs: { src: result.data.url, width: 100, intrinsicWidth: result.data.width, intrinsicHeight: result.data.height } }, { type: "paragraph" }]).run();
  } catch (err) { error.value = message(err); }
  finally { uploading.value = null; editor.value?.setEditable(!props.busy, false); input.value = ""; }
}
async function deleteMedia(type: MediaNodeType, src: string) {
  if (!editor.value || props.busy || uploading.value || deletingMedia.value) return;
  deletingMedia.value = true;
  editor.value.setEditable(false, false);
  error.value = "";
  try {
    const parts = new URL(src).pathname.split("/");
    const filename = parts.pop();
    if (!filename || parts.pop() !== mediaSettings[type].folder) throw new Error("Bu dosya silinemiyor.");
    await api(`/admin/blog/${mediaSettings[type].folder}/${encodeURIComponent(filename)}`, send("DELETE"));
    deletedMedia.add(src);
    const transaction = editor.value.state.tr;
    // Remove every occurrence of the deleted file in this draft.
    editor.value.state.doc.descendants((node, pos) => {
      if (node.type.name === type && node.attrs.src === src) {
        transaction.delete(transaction.mapping.map(pos), transaction.mapping.map(pos + node.nodeSize));
      }
    });
    editor.value.view.dispatch(transaction);
  } catch (err) { error.value = message(err); }
  finally { deletingMedia.value = false; editor.value?.setEditable(!props.busy, false); }
}
const editor = useEditor({
  extensions: [...[BlogImage, BlogVideo, BlogAudio].map(node => node.configure({ isDeleted: (src: string) => deletedMedia.has(src) })), Document, Paragraph, Text, HardBreak, UndoRedo, Placeholder.configure({ placeholder: "Yazmaya başlayın…" })],
  content: { type: "doc", content: readPostBody(form.body) },
  editable: !props.busy,
  editorProps: {
    attributes: { role: "textbox", "aria-label": "İçerik", "aria-multiline": "true", "aria-required": "true", spellcheck: "false", lang: "tr" },
    handleClickOn(view, _pos, node, nodePos, event) {
      if (node.type.name !== "blogImage" || !(event.target instanceof HTMLImageElement)) return false;
      view.dispatch(view.state.tr.setSelection(NodeSelection.create(view.state.doc, nodePos)));
      return true;
    },
    handlePaste(_view, event) {
      const text = event.clipboardData?.getData("text/plain");
      if (!text) return false;
      if (activeMedia.value) return false;
      editor.value?.commands.insertContent(paragraphsToDocument(text));
      return true;
    },
  },
  onUpdate({ editor }) {
    form.body = writePostBody(editor.getJSON().content ?? []);
    error.value = "";
  },
});
const activeMedia = computed(() => mediaNodeTypes.find(type => editor.value?.isActive(type)));
watch(() => props.busy, (busy) => editor.value?.setEditable(!busy && !uploading.value && !deletingMedia.value, false));
onMounted(() => { if (!props.post) titleField.value?.focus(); });

function updateTitle(event: Event) {
  const field = event.currentTarget as HTMLTextAreaElement;
  field.value = field.value.replaceAll("\n", " ");
  form.title = field.value;
  error.value = "";
}
function suggestSlug() {
  if (!form.slug)
    form.slug = form.title
      .toLowerCase()
      .normalize("NFKD")
      .replaceAll("ı", "i")
      .replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "");
}
async function save() {
  if (props.busy || uploading.value || deletingMedia.value) return;
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
  if (!editor.value?.getText().trim() && !editor.value?.getJSON().content?.some(node => isMediaNode(node.type))) {
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
  <form ref="formElement" class="post-editor" lang="tr" spellcheck="false" novalidate @submit.prevent="save">
    <div class="post-editor-toolbar">
      <label for="blog-image-file" hidden>Yazıya eklenecek görsel</label>
      <input id="blog-image-file" ref="imageInput" type="file" accept="image/jpeg,image/png,image/gif" hidden @change="uploadMedia($event, 'blogImage')" />
      <label for="blog-video-file" hidden>Yazıya eklenecek video</label>
      <input id="blog-video-file" ref="videoInput" type="file" accept="video/mp4,.mp4" hidden @change="uploadMedia($event, 'blogVideo')" />
      <label for="blog-audio-file" hidden>Yazıya eklenecek ses</label>
      <input id="blog-audio-file" ref="audioInput" type="file" accept=".opus,.ogg,audio/ogg" hidden @change="uploadMedia($event, 'blogAudio')" />
      <span class="hint">{{ post ? "Yazıyı düzenle" : "Yeni yazı" }}</span>
      <div class="actions">
        <button type="button" :disabled="busy || !!uploading || deletingMedia" @click="imageInput?.click()">{{ uploading === "blogImage" ? "Yükleniyor…" : "Görsel ekle" }}</button>
        <button type="button" :disabled="busy || !!uploading || deletingMedia" :title="`MP4 · AV1 · İsteğe bağlı Opus ses · En fazla ${mediaSettings.blogVideo.maxMiB} MiB`" @click="videoInput?.click()">{{ uploading === "blogVideo" ? "Yükleniyor…" : "Video ekle" }}</button>
        <button type="button" :disabled="busy || !!uploading || deletingMedia" :title="`Ogg Opus · En fazla ${mediaSettings.blogAudio.maxMiB} MiB`" @click="audioInput?.click()">{{ uploading === "blogAudio" ? "Yükleniyor…" : "Ses ekle" }}</button>
        <button type="button" :disabled="busy || !!uploading || deletingMedia" :aria-expanded="settingsOpen" aria-controls="post-settings" @click="settingsOpen = !settingsOpen">Yazı ayarları</button>
        <button type="submit" :disabled="busy || !!uploading || deletingMedia">Yazıyı kaydet</button>
        <button type="button" :disabled="busy || !!uploading || deletingMedia" @click="emit('cancel')">Vazgeç</button>
      </div>
      <div v-if="editor && activeMedia" class="image-size-controls" :aria-label="mediaSettings[activeMedia].width">
        <span>{{ mediaSettings[activeMedia].width }}</span>
        <button v-for="width in [25, 50, 75, 100]" :key="width" type="button" :disabled="busy || !!uploading || deletingMedia" :aria-pressed="editor.getAttributes(activeMedia).width === width" @click="editor.chain().focus().updateAttributes(activeMedia, { width }).run()">%{{ width }}</button>
        <button type="button" class="image-delete-control" :title="mediaSettings[activeMedia].deleteTitle" :disabled="busy || !!uploading || deletingMedia" @mousedown.prevent @click="deleteMedia(activeMedia, editor.getAttributes(activeMedia).src)">{{ mediaSettings[activeMedia].remove }}</button>
      </div>
    </div>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <fieldset v-show="settingsOpen" id="post-settings" class="post-settings" :disabled="busy || !!uploading || deletingMedia">
      <label>Kısa bağlantı<input v-model="form.slug" required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="200" placeholder="ilk-yazim" /></label>
      <label>Özet<textarea v-model="form.excerpt" rows="2" maxlength="2000"></textarea></label>
      <label>Durum<select v-model="form.published">
        <option :value="0">Taslak — yalnızca yönetim panelinde görünür</option>
        <option :value="1">Yayında — herkese açık</option>
      </select></label>
    </fieldset>
    <PostArticle :title="form.title" :body="form.body" :date="date">
      <template #title>
        <textarea ref="titleField" class="editable-title" :value="form.title" rows="1" required
          aria-label="Başlık" :disabled="busy" placeholder="Yazının başlığı" lang="tr" spellcheck="false"
          @input="updateTitle" @blur="suggestSlug" @keydown.enter.prevent="editor?.commands.focus()"></textarea>
      </template>
      <template #body><EditorContent :editor="editor" class="post-body" /></template>
    </PostArticle>
  </form>
</template>
