import '../../app.css';
import { mount } from 'svelte';
import TranslateWindow from '../../components/TranslateWindow.svelte';
import { initTheme } from '../../stores/theme';
import { initWindow } from '../../stores/ui';
import { installFrontendLogging } from '../../runtime/frontendLog';
import { AppService } from '@bindings/cnb.cool/dtapp/kai/internal/service';

installFrontendLogging();
initTheme();
initWindow();

// 把 WebView 的 UA 传给后端，作为全局 HTTP 请求默认 User-Agent
AppService.SetUserAgent(navigator.userAgent);
console.log('UA:', navigator.userAgent);

const app = mount(TranslateWindow, {
  target: document.getElementById('app')!,
});

export default app;
