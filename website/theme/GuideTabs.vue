<script setup>
import { ref } from 'vue';

const props = defineProps({example: {type: String, required: true}});
const examples = {
  'panel-focus': {
    title: 'Panel focus diagram',
    tabs: [
      {label: 'Sidebar', text: 'Choose a workspace or resource type.'},
      {label: 'Main', text: 'Browse resources and open details.'},
      {label: 'Assistant', text: 'Chat with the selected agent.'},
    ],
  },
  'mcp-target': {
    title: 'Where an MCP request goes',
    tabs: [
      {
        label: 'Request path',
        detail: 'Coding agent → Plous MCP → Kubernetes context',
        text: 'Plous reads from the Kubernetes context selected for the request.',
      },
      {
        label: 'Example target',
        detail: 'kubernetes_context_read → minikube / shop',
        text: 'Check the context and default namespace before asking your agent to inspect workloads.',
      },
    ],
  },
};
const {title, tabs} = examples[props.example];
const selected = ref(0);
const id = (role, index) => `${props.example}-${role}-${index}`;

function moveFocus(event, index) {
  const last = tabs.length - 1;
  const destinations = {ArrowLeft: (index + last) % tabs.length, ArrowRight: (index + 1) % tabs.length, Home: 0, End: last};
  if (!Object.hasOwn(destinations, event.key)) return;
  event.preventDefault();
  selected.value = destinations[event.key];
  event.currentTarget.parentElement.querySelectorAll('button')[selected.value].focus();
}
</script>

<template>
  <section class="guide-tabs" :aria-label="title">
    <div role="tablist" :aria-label="title">
      <button v-for="(tab, index) in tabs" :key="tab.label" :id="id('tab', index)"
        type="button" role="tab" :aria-controls="id('panel', index)"
        :aria-selected="index === selected" :tabindex="index === selected ? 0 : -1"
        @click="selected = index" @keydown="moveFocus($event, index)">
        {{ tab.label }}
      </button>
    </div>
    <div v-for="(tab, index) in tabs" :key="tab.label" :id="id('panel', index)"
      role="tabpanel" :aria-labelledby="id('tab', index)"
      :hidden="index !== selected" :tabindex="index === selected ? 0 : -1">
      <code v-if="tab.detail">{{ tab.detail }}</code>
      <p>{{ tab.text }}</p>
    </div>
  </section>
</template>
