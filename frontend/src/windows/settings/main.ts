import '../../app.css';
import { mount } from 'svelte';
import Settings from '../../components/Settings.svelte';
import { initTheme } from '../../stores/theme';
import { initWindow } from '../../stores/ui';
import { installFrontendLogging } from '../../runtime/frontendLog';
import { AppService } from '@bindings/cnb.cool/dtapp/kai/internal/service';

installFrontendLogging();
initTheme();
initWindow();

// 把 WebView 的 UA 传给后端，作为全局 HTTP 请求默认 User-Agent
AppService.SetUserAgent(navigator.userAgent);
console.debug('UA:', navigator.userAgent);

const app = mount(Settings, {
  target: document.getElementById('app')!,
});

export default app;
