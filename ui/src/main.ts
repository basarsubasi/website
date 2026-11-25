import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import App from "./app/App.vue";
import Gallery from "./pages/gallery/Gallery.vue";
import Photo from "./pages/gallery/Photo.vue";
import Blog from "./pages/blog/Blog.vue";
import NotFound from "./pages/NotFound.vue";
import "./styles/gallery.css";
import "./styles/site.css";
const router = createRouter({
  history: createWebHistory(),
  scrollBehavior: () => ({ top: 0 }),
  routes: [
    { path: "/", alias: "/photos", component: Gallery },
    { path: "/bw-photos", component: Gallery },
    { path: "/color-photos", component: Gallery },
    { path: "/photo/:uuid", component: Photo },
    { path: "/blog", component: Blog },
    { path: "/blog/:slug", component: Blog },
    { path: "/admin", component: () => import("./pages/admin/Admin.vue") },
    { path: "/:pathMatch(.*)*", component: NotFound },
  ],
});
createApp(App).use(router).mount("#app");
