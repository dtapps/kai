<script lang="ts">
  import { Window, emitEvent } from '../runtime';
  import { t } from '../i18n';
  import { isDark } from '../stores/theme';
  import { EventWindowClosing } from '../utils/events';
  import { isMac } from '../runtime/platform';

  let { onClose }: { onClose?: () => void } = $props();

  // 平台判断统一走 runtime/platform（Wails v3 多窗口下 _wails 可能丢失，需 UA 兜底）。
  const isMacPlatform = isMac();
  console.debug('[TitleBar] isMac =', isMacPlatform);

  function minimize() {
    Window.Minimise();
  }
  function toggleMax() {
    Window.ToggleMaximise();
  }
  function close() {
    // 自定义关闭行为（如截图窗口仅隐藏不销毁）；否则走标准关闭流程。
    if (onClose) {
      onClose();
      return;
    }
    emitEvent(EventWindowClosing);
    Window.Close();
  }
</script>

<div
  class="u-titlebar u-drag flex h-9 items-center select-none px-2 {isMacPlatform
    ? 'justify-start'
    : 'justify-end'}"
  class:dark={$isDark}
>
  {#if isMacPlatform}
    <div class="u-no-drag flex gap-2 pr-2">
      <button class="u-traffic u-traffic--close" aria-label={t('titlebar.close')} onclick={close}
      ></button>
      <button
        class="u-traffic u-traffic--min"
        aria-label={t('titlebar.minimize')}
        onclick={minimize}
      ></button>
      <button
        class="u-traffic u-traffic--max"
        aria-label={t('titlebar.maximize')}
        onclick={toggleMax}
      ></button>
    </div>
  {:else}
    <div class="u-no-drag flex gap-0.5">
      <button class="u-winbtn" aria-label={t('titlebar.minimize')} onclick={minimize}>—</button>
      <button class="u-winbtn" aria-label={t('titlebar.maximize')} onclick={toggleMax}>▢</button>
      <button class="u-winbtn u-winbtn--close" aria-label={t('titlebar.close')} onclick={close}
        >✕</button
      >
    </div>
  {/if}
</div>
