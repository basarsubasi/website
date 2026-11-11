<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api, ApiError, send, message } from "./api";
import type {
  Session,
  Photo,
  PhotoInput,
  Post,
  PostInput,
  ListResponse,
} from "./types";
import PhotoEditor from "./PhotoEditor.vue";
import PostEditor from "./PostEditor.vue";
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
  photo.value = value;
  editing.value = true;
  error.value = "";
  notice.value = "";
  deleting.value = null;
}
function editPost(value: Post | null) {
  post.value = value;
  editing.value = true;
  error.value = "";
  notice.value = "";
  deleting.value = null;
}
async function savePhoto(value: PhotoInput, file: File | null) {
  busy.value = true;
  error.value = "";
  try {
    if (photo.value) {
      await api("/admin/images/" + photo.value.uuid, send("PUT", value));
    } else {
      if (!file) throw new Error("Choose a photo file to upload.");
      const form = new FormData();
      form.append("photo", file);
      form.append("metadata", JSON.stringify(value));
      await api("/admin/images", { method: "POST", body: form });
    }
    editing.value = false;
    notice.value = "Photo saved.";
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
  try {
    await api(
      "/admin/posts" + (post.value ? "/" + post.value.id : ""),
      send(post.value ? "PUT" : "POST", value),
    );
    editing.value = false;
    notice.value = "Post saved.";
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
    notice.value = "Deleted.";
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
  <section class="admin">
    <p v-if="checking" role="status">Checking session…</p>
    <template v-else>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <p v-if="notice" class="success" role="status">{{ notice }}</p>
      <form v-if="!authenticated" class="login" @submit.prevent="login">
        <h1>Admin</h1>
        <label
          >Password<input
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
            :disabled="busy" /></label
        ><button :disabled="busy">
          {{ busy ? "Signing in…" : "Sign in" }}
        </button>
      </form>
      <template v-else>
        <div class="admin-heading">
          <h1>Admin</h1>
          <button :disabled="busy || loading" @click="logout">Sign out</button>
        </div>
        <nav class="admin-tabs" aria-label="Content type">
          <button
            :aria-pressed="tab === 'photos'"
            :disabled="busy || loading"
            @click="changeTab('photos')"
          >
            Photos</button
          ><button
            :aria-pressed="tab === 'posts'"
            :disabled="busy || loading"
            @click="changeTab('posts')"
          >
            Blog posts
          </button>
        </nav>
        <PhotoEditor
          v-if="editing && tab === 'photos'"
          :key="photo?.uuid ?? 'new-photo'"
          :photo="photo"
          :busy="busy"
          @save="savePhoto"
          @cancel="editing = false"
        />
        <PostEditor
          v-else-if="editing && tab === 'posts'"
          :key="post?.id ?? 'new-post'"
          :post="post"
          :busy="busy"
          @save="savePost"
          @cancel="editing = false"
        />
        <template v-else>
          <div class="actions">
            <button
              :disabled="busy || loading"
              @click="tab === 'photos' ? editPhoto(null) : editPost(null)"
            >
              {{ tab === "photos" ? "Add photo" : "New post" }}</button
            ><button :disabled="busy || loading" @click="load">Refresh</button>
          </div>
          <p v-if="loading" role="status">Loading…</p>
          <template v-else
            ><p v-if="!total">
              {{ tab === "photos" ? "No photos yet." : "No posts yet." }}
            </p>
            <ul v-if="tab === 'photos'" class="admin-list">
              <li v-for="item in photos" :key="item.uuid">
                <img
                  :src="item.image_url"
                  :alt="item.name || 'photo'"
                  loading="lazy"
                />
                <div class="record-info">
                  <strong>{{ item.name || "Untitled photo" }}</strong
                  ><span
                    >{{ item.color }} ·
                    {{
                      [item.city, item.country].filter(Boolean).join(", ")
                    }}</span
                  >
                </div>
                <div class="actions">
                  <button :disabled="busy" @click="editPhoto(item)">Edit</button
                  ><button :disabled="busy" @click="deleting = item.uuid">
                    Delete
                  </button>
                </div>
                <div
                  v-if="deleting === item.uuid"
                  class="delete-confirm"
                  role="alert"
                >
                  Delete this photo?<button
                    :disabled="busy"
                    @click="remove(item.uuid)"
                  >
                    Delete permanently</button
                  ><button :disabled="busy" @click="deleting = null">
                    Cancel
                  </button>
                </div>
              </li>
            </ul>
            <ul v-else class="admin-list">
              <li v-for="item in posts" :key="item.id">
                <div class="record-info">
                  <strong>{{ item.title }}</strong
                  ><span
                    >{{ item.published ? "Published" : "Draft" }} · /blog/{{
                      item.slug
                    }}</span
                  >
                </div>
                <div class="actions">
                  <button :disabled="busy" @click="editPost(item)">Edit</button
                  ><RouterLink v-if="item.published" :to="'/blog/' + item.slug"
                    >View</RouterLink
                  ><button :disabled="busy" @click="deleting = item.id">
                    Delete
                  </button>
                </div>
                <div
                  v-if="deleting === item.id"
                  class="delete-confirm"
                  role="alert"
                >
                  Delete this post?<button
                    :disabled="busy"
                    @click="remove(item.id)"
                  >
                    Delete permanently</button
                  ><button :disabled="busy" @click="deleting = null">
                    Cancel
                  </button>
                </div>
              </li>
            </ul>
            <div v-if="total > 20" class="pagination">
              <button
                :disabled="busy || page === 1"
                @click="changePage(page - 1)"
              >
                Previous</button
              ><span>{{ page }}</span
              ><button
                :disabled="busy || page * 20 >= total"
                @click="changePage(page + 1)"
              >
                Next
              </button>
            </div>
          </template>
        </template>
      </template>
    </template>
  </section>
</template>
