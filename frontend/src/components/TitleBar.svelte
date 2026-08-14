<script lang="ts">
  import { Window, emitEvent } from '../runtime';
  import { t } from '../i18n';
  import { isDark } from '../stores/theme';
  import { EventWindowClosing } from '../utils/events';
  import { System } from '@wailsio/runtime';

  let { onClose }: { onClose?: () => void } = $props();

  // 使用 Wails v3 的 System.IsMac() 判断平台（优于已废弃的 navigator.platform）
  const isMac = System.IsMac();

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
  class="u-titlebar u-drag flex h-9 items-center select-none px-2 {isMac
    ? 'justify-start'
    : 'justify-end'}"
  class:dark={$isDark}
>
  {#if isMac}
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
    <div class="u-no-drag flex gap-1">
      <button class="u-winbtn" aria-label={t('titlebar.minimize')} onclick={minimize}>—</button>
      <button class="u-winbtn" aria-label={t('titlebar.maximize')} onclick={toggleMax}>▢</button>
      <button class="u-winbtn u-winbtn--close" aria-label={t('titlebar.close')} onclick={close}
        >✕</button
      >
    </div>
  {/if}
</div>
