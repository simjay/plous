<script setup>
import { withBase } from 'vitepress';

const image = withBase('/assets/plous-overview.png');
</script>

<template>
  <figure class="workspace-preview">
    <a :href="image" aria-label="View the Plous workspace screenshot at full size">
      <img :src="image" width="3024" height="1898"
        alt="Plous terminal showing the connections tree, cluster overview, and Codex assistant." />
    </a>
    <figcaption>Workspace overview from an earlier Plous build</figcaption>
  </figure>
</template>
