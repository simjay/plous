<script setup>
import { onMounted, ref } from 'vue';
import { useData, useRoute } from 'vitepress';

const { site, theme } = useData();
const route = useRoute();
const current = theme.value.documentationVersion;
const options = ref([{version: current}]);
const latest = ref('');

onMounted(async () => {
  try {
    const response = await fetch(theme.value.versionsURL);
    if (!response.ok) return;
    const catalog = await response.json();
    if (!catalog.versions.some(entry => entry.version === current)) return;
    options.value = catalog.versions;
    latest.value = catalog.latest;
  } catch {
    // Standalone previews have only the current version.
  }
});

function switchVersion(event) {
  const selected = options.value.find(entry => entry.version === event.target.value);
  if (!selected?.base || !selected.routes) return;
  const path = route.path.slice(site.value.base.length).replace(/index(?:\.html)?$/, '');
  const exists = selected.routes.includes(path);
  window.location.assign(selected.base + (exists ? path + location.search + location.hash : 'docs/'));
}
</script>

<template>
  <select class="version-picker" aria-label="Documentation version" :value="current"
    :disabled="options.length < 2" @change="switchVersion">
    <option v-for="entry in options" :key="entry.version" :value="entry.version">
      {{ entry.version }}{{ entry.version === latest ? ' (latest)' : '' }}
    </option>
  </select>
</template>
