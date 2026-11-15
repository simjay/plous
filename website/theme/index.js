import { h } from 'vue';
import DefaultTheme from 'vitepress/theme';
import GuideTabs from './GuideTabs.vue';
import VersionPicker from './VersionPicker.vue';
import WorkspaceScreenshot from './WorkspaceScreenshot.vue';
import './style.css';

export default {
  extends: DefaultTheme,
  Layout() {
    return h(DefaultTheme.Layout, null, {
      'nav-bar-content-after': () => h(VersionPicker, {class: 'desktop-version'}),
      'nav-screen-content-after': () => h(VersionPicker, {class: 'mobile-version'}),
    });
  },
  enhanceApp({app}) {
    app.component('GuideTabs', GuideTabs);
    app.component('WorkspaceScreenshot', WorkspaceScreenshot);
  },
};
