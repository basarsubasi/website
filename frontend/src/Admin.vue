<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, watch } from "vue";
import { api, ApiError, send, message } from "./api";
import type {
  Session,
  Photo,
  PhotoInput,
  Post,
  PostInput,
  ListResponse,
} from "./types";
import { onBeforeRouteLeave } from "vue-router";
import PhotoEditor from "./PhotoEditor.vue";
const PostEditor = defineAsyncComponent(() => import("./PostEditor.vue"));
const authenticated = ref(false),
  checking = ref(true),
  password = ref(""),
  busy = ref(false),
  loading = ref(false),
  error = ref(""),
  notice = ref("");
const tab = ref<"photos" | "posts">("photos"),
  photos = ref<Photo[]>([]),
  posts = ref<Post[]>([]),
  page = ref(1),
  total = ref(0);
const editing = ref(false),
  photo = ref<Photo | null>(null),
  post = ref<Post | null>(null),
  deleting = ref<string | null>(null);
const editorDirty = ref(false);
const hasUnsavedChanges = computed(() => editing.value && (editorDirty.value || busy.value));
function confirmLeave() {
  return !hasUnsavedChanges.value || window.confirm("Kaydedilmemiş değişiklikleriniz var. Kaydetmeden çıkmak istediğinizden emin misiniz?");
}
function beforeUnload(event: BeforeUnloadEvent) {
  if (!hasUnsavedChanges.value) return;
  event.preventDefault();
  event.returnValue = "";
}
watch(hasUnsavedChanges, active => {
  if (active) window.addEventListener("beforeunload", beforeUnload);
  else window.removeEventListener("beforeunload", beforeUnload);
}, { flush: "sync" });
onUnmounted(() => window.removeEventListener("beforeunload", beforeUnload));
onBeforeRouteLeave(() => confirmLeave());
function cancelEdit() {
  if (confirmLeave()) editing.value = false;
}
function handleError(e: unknown) {
  error.value = message(e);
  if (e instanceof ApiError && e.status === 401) authenticated.value = false;
}
async function load() {
  loading.value = true;
  deleting.value = null;
  try {
    if (tab.value === "photos") {
      const result = await api<ListResponse<Photo>>(
        `/images?page=${page.value}&limit=20`,
      );
      photos.value = result.data;
      total.value = result.total;
    } else {
      const result = await api<ListResponse<Post>>(
        `/admin/posts?page=${page.value}&limit=20`,
      );
      posts.value = result.data;
      total.value = result.total;
    }
  } catch (e) {
    handleError(e);
  } finally {
    loading.value = false;
  }
}
onMounted(async () => {
  try {
    authenticated.value = (await api<Session>("/auth/session")).authenticated;
    if (authenticated.value) await load();
  } catch (e) {
    handleError(e);
  } finally {
    checking.value = false;
  }
});
async function login() {
  busy.value = true;
  error.value = "";
  try {
    await api<Session>(
      "/auth/login",
      send("POST", { password: password.value }),
    );
    password.value = "";
    authenticated.value = true;
    await load();
  } catch (e) {
    handleError(e);
  } finally {
    busy.value = false;
  }
}
async function logout() {
  if (!confirmLeave()) return;
  busy.value = true;
  error.value = "";
  try {
    await api<void>("/auth/logout", send("POST"));
    authenticated.value = false;
    editing.value = false;
    photos.value = [];
    posts.value = [];
    notice.value = "";
  } catch (e) {
    handleError(e);
  } finally {
    busy.value = false;
  }
}
async function changeTab(value: "photos" | "posts") {
  if (!confirmLeave()) return;
  tab.value = value;
  page.value = 1;
  editing.value = false;
  error.value = "";
  notice.value = "";
  await load();
}
async function changePage(value: number) {
  page.value = value;
  error.value = "";
  await load();
}
function editPhoto(value: Photo | null) {
  editorDirty.value = false;
  photo.value = value;
  editing.value = true;
  error.value = "";
  notice.value = "";
  deleting.value = null;
}
function editPost(value: Post | null) {
  editorDirty.value = false;
  post.value = value;
  editing.value = true;
  error.value = "";
  notice.value = "";
  deleting.value = null;
}
async function savePhoto(value: PhotoInput, file: File | null) {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    if (photo.value) {
      await api("/admin/images/" + photo.value.uuid, send("PUT", value));
    } else {
      if (!file) throw new Error("Yüklemek için bir fotoğraf seçin.");
      const form = new FormData();
      form.append("photo", file);
      form.append("metadata", JSON.stringify(value));
      await api("/admin/images", { method: "POST", body: form });
    }
    editing.value = false;
    notice.value = "Fotoğraf kaydedildi.";
    await load();
  } catch (e) {
    handleError(e);
  } finally {
    busy.value = false;
  }
}
async function movePhoto(item: Photo, index: number, direction: "up" | "down") {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await api("/admin/images/" + item.uuid + "/move", send("POST", { direction }));
    // Follow the photo when it moves across a page boundary.
    if (direction === "up" && index === 0 && page.value > 1) page.value--;
    else if (direction === "down" && index === 19) page.value++;
    notice.value = "Fotoğraf sırası kaydedildi.";
    await load();
  } catch (e) {
    handleError(e);
  } finally {
    busy.value = false;
  }
}
async function savePost(value: PostInput) {
  busy.value = true;
  error.value = "";
  notice.value = "";
  try {
    await api(
      "/admin/posts" + (post.value ? "/" + post.value.id : ""),
      send(post.value ? "PUT" : "POST", value),
    );
    editing.value = false;
    notice.value = "Yazı kaydedildi.";
    await load();
  } catch (e) {
    handleError(e);
  } finally {
    busy.value = false;
  }
}
async function remove(id: string) {
  busy.value = true;
  error.value = "";
  try {
    await api(
      "/admin/" + (tab.value === "photos" ? "images" : "posts") + "/" + id,
      send("DELETE"),
    );
    notice.value = "Silindi.";
    if (
      (tab.value === "photos" ? photos.value.length : posts.value.length) ===
        1 &&
      page.value > 1
    )
      page.value--;
    await load();
  } catch (e) {
    handleError(e);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section class="admin" :class="{ 'admin--writing': editing && tab === 'posts' }" :aria-busy="checking || loading || busy">
    <template v-if="!checking">
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-if="notice" class="success" role="status">{{ notice }}</p>
      <form v-if="!authenticated" class="login" @submit.prevent="login">
        <h1>Yönetim</h1>
        <label
          >Şifre<input
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
            :disabled="busy" /></label
        ><button :disabled="busy">
          Giriş yap
        </button>
      </form>
      <template v-else>
        <div v-if="!(editing && tab === 'posts')" class="admin-heading">
          <h1>Yönetim</h1>
          <button :disabled="busy || loading" @click="logout">Çıkış yap</button>
        </div>
        <nav v-if="!(editing && tab === 'posts')" class="admin-tabs" aria-label="İçerik türü">
          <button
            :aria-pressed="tab === 'photos'"
            :disabled="busy || loading"
            @click="changeTab('photos')"
          >
            Fotoğraflar</button
          ><button
            :aria-pressed="tab === 'posts'"
            :disabled="busy || loading"
            @click="changeTab('posts')"
          >
            Blog yazıları
          </button>
        </nav>
        <PhotoEditor
          v-if="editing && tab === 'photos'"
          :key="photo?.uuid ?? 'new-photo'"
          :photo="photo"
          :busy="busy"
          @save="savePhoto"
          @cancel="cancelEdit"
          @dirty="editorDirty = $event"
        />
        <PostEditor
          v-else-if="editing && tab === 'posts'"
          :key="post?.id ?? 'new-post'"
          :post="post"
          :busy="busy"
          @save="savePost"
          @cancel="cancelEdit"
          @dirty="editorDirty = $event"
        />
        <template v-else>
          <div class="actions">
            <button
              :disabled="busy || loading"
              @click="tab === 'photos' ? editPhoto(null) : editPost(null)"
            >
              {{ tab === "photos" ? "Fotoğraf ekle" : "Yeni yazı" }}</button
            ><button :disabled="busy || loading" @click="load">Yenile</button>
          </div>
          <template v-if="!loading"
            ><p v-if="!total">
              {{ tab === "photos" ? "Henüz fotoğraf yok." : "Henüz yazı yok." }}
            </p>
            <ul v-if="tab === 'photos'" class="admin-list">
              <li v-for="(item, index) in photos" :key="item.uuid">
                <img
                  :src="item.image_url"
                  :alt="item.name || 'Fotoğraf'"
                  loading="lazy"
                />
                <div class="record-info">
                  <strong>{{ item.name || "Adsız fotoğraf" }}</strong
                  ><span
                    >{{ item.color === "bw" ? "Siyah beyaz" : "Renkli" }} ·
                    {{
                      [item.city, item.country].filter(Boolean).join(", ")
                    }}</span
                  >
                </div>
                <div class="actions">
                  <button
                    aria-label="Yukarı taşı" title="Yukarı taşı"
                    :disabled="busy || (page === 1 && index === 0)"
                    @click="movePhoto(item, index, 'up')"
                  >↑</button>
                  <button
                    aria-label="Aşağı taşı" title="Aşağı taşı"
                    :disabled="busy || (page - 1) * 20 + index + 1 >= total"
                    @click="movePhoto(item, index, 'down')"
                  >↓</button>
                  <button :disabled="busy" @click="editPhoto(item)">Düzenle</button
                  ><button :disabled="busy" @click="deleting = item.uuid">
                    Sil
                  </button>
                </div>
                <div
                  v-if="deleting === item.uuid"
                  class="delete-confirm"
                  role="alert"
                >
                  Bu fotoğraf silinsin mi?<button
                    :disabled="busy"
                    @click="remove(item.uuid)"
                  >
                    Kalıcı olarak sil</button
                  ><button :disabled="busy" @click="deleting = null">
                    Vazgeç
                  </button>
                </div>
              </li>
            </ul>
            <ul v-else class="admin-list">
              <li v-for="item in posts" :key="item.id">
                <div class="record-info">
                  <strong>{{ item.title }}</strong
                  ><span
                    >{{ item.published ? "Yayında" : "Taslak" }} · /blog/{{
                      item.slug
                    }}</span
                  >
                </div>
                <div class="actions">
                  <button :disabled="busy" @click="editPost(item)">Düzenle</button
                  ><RouterLink v-if="item.published" :to="'/blog/' + item.slug"
                    >Görüntüle</RouterLink
                  ><button :disabled="busy" @click="deleting = item.id">
                    Sil
                  </button>
                </div>
                <div
                  v-if="deleting === item.id"
                  class="delete-confirm"
                  role="alert"
                >
                  Bu yazı silinsin mi?<button
                    :disabled="busy"
                    @click="remove(item.id)"
                  >
                    Kalıcı olarak sil</button
                  ><button :disabled="busy" @click="deleting = null">
                    Vazgeç
                  </button>
                </div>
              </li>
            </ul>
            <div v-if="total > 20" class="pagination">
              <button
                :disabled="busy || page === 1"
                @click="changePage(page - 1)"
              >
                Önceki</button
              ><span>{{ page }}</span
              ><button
                :disabled="busy || page * 20 >= total"
                @click="changePage(page + 1)"
              >
                Sonraki
              </button>
            </div>
          </template>
        </template>
      </template>
    </template>
  </section>
</template>
