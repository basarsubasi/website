<script setup lang="ts">
import { reactive, ref, onUnmounted } from "vue";
import type { Photo, PhotoInput } from "./types";
const props = defineProps<{ photo: Photo | null; busy: boolean }>();
const emit = defineEmits<{
  save: [value: PhotoInput, file: File | null];
  cancel: [];
}>();
const form = reactive<PhotoInput>({
  color: props.photo?.color ?? "color",
  name: props.photo?.name ?? "",
  country: props.photo?.country ?? "",
  city: props.photo?.city ?? "",
  year_taken: props.photo?.year_taken ?? 0,
});
const file = ref<File | null>(null),
  preview = ref(""),
  error = ref("");
function chooseFile(event: Event) {
  if (preview.value) URL.revokeObjectURL(preview.value);
  file.value = null;
  preview.value = "";
  error.value = "";
  const input = event.target as HTMLInputElement;
  const selected = input.files?.[0];
  if (!selected) return;
  if (selected.size > 25 * 1024 * 1024) {
    error.value = "Fotoğraf en fazla 25 MiB olabilir.";
    input.value = "";
    return;
  }
  file.value = selected;
  preview.value = URL.createObjectURL(selected);
}
onUnmounted(() => {
  if (preview.value) URL.revokeObjectURL(preview.value);
});
const textFields = [
  ["name", "Ad / alternatif metin"],
  ["city", "Şehir"],
  ["country", "Ülke"],
] as const;
</script>
<template>
  <form class="editor" @submit.prevent="emit('save', { ...form }, file)">
    <h2>{{ photo ? "Fotoğrafı düzenle" : "Fotoğraf ekle" }}</h2>
    <fieldset :disabled="busy">
      <template v-if="!photo">
        <label
          >Fotoğraf dosyası<input
            type="file"
            accept="image/jpeg,image/png,image/gif"
            required
            @change="chooseFile"
        /></label>
        <p class="hint">JPEG, PNG veya GIF. En fazla 25 MiB.</p>
      </template>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <img
        v-if="preview || photo"
        class="upload-preview"
        :src="preview || photo?.image_url"
        :alt="form.name || 'Fotoğraf önizlemesi'"
      />
      <div class="form-grid">
        <label
          >Renk<select v-model="form.color">
            <option value="color">Renkli</option>
            <option value="bw">Siyah beyaz</option>
          </select></label
        >
        <label v-for="[key, label] in textFields" :key="key"
          >{{ label }}<input v-model="form[key]" type="text"
        /></label>
        <label
          >Yıl<input
            v-model.number="form.year_taken"
            type="number"
            min="0"
            max="9999"
            step="1"
            required
        /></label>
      </div>
      <div class="actions">
        <button type="submit" :disabled="!photo && !file">
          Fotoğrafı kaydet</button
        ><button type="button" @click="emit('cancel')">Vazgeç</button>
      </div>
    </fieldset>
  </form>
</template>
<style scoped>
.upload-preview {
  max-height: 260px;
  width: auto;
  max-width: 100%;
  margin: 20px 0;
  object-fit: contain;
}
</style>
