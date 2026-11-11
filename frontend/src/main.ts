import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";
import Gallery from "./Gallery.vue";
import Photo from "./Photo.vue";
import Blog from "./Blog.vue";
import NotFound from "./NotFound.vue";
import "./gallery.css";
import "./site.css";
const router = createRouter({
  history: createWebHistory(),
  scrollBehavior: () => ({ top: 0 }),
  routes: [
    { path: "/gallery", component: Gallery },
    { path: "/bw-photos", component: Gallery },
    { path: "/color-photos", component: Gallery },
    { path: "/image/:uuid", component: Photo },
    { path: "/", alias: "/blog", component: Blog },
    { path: "/blog/:slug", component: Blog },
    { path: "/admin", component: () => import("./Admin.vue") },
    { path: "/:pathMatch(.*)*", component: NotFound },
  ],
});
createApp(App).use(router).mount("#app");
