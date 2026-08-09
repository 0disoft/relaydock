import { mount } from 'svelte';
import 'virtual:uno.css';
import './styles.css';
import App from './App.svelte';

const target = document.getElementById('app');
if (!target) {
  throw new Error('App mount element was not found');
}

const app = mount(App, { target });
export default app;
