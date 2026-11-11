<template>
  <div class="layout">
    <header class="page-header">
      <RouterLink to="/" class="main-title"
        >basarsubasi's <span class="title-photos">photos</span></RouterLink
      >
      <RouterLink to="/bw-photos">b&w</RouterLink>
      <RouterLink to="/color-photos">color</RouterLink>
      <RouterLink to="/blog">blog</RouterLink>
    </header>
    <main class="content"><RouterView /></main>
    <footer class="page-footer">
      {{ new Date().getFullYear() }} © başar subaşı |
      <a
        href="https://github.com/basarsubasi/website"
        target="_blank"
        rel="noopener noreferrer"
        >source code</a
      >
      | <RouterLink to="/admin">admin</RouterLink>
    </footer>
  </div>
</template>
