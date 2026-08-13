<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from 'svelte/store';
  import TitleBar from './TitleBar.svelte';
  import { t, locale } from '../i18n';
  import { rootStyle } from '../stores/theme';
  import { rootStyleToStyle } from '../utils/style';
  import { Lang, type LangCode } from '../constants/lang';
  import GeneralTab from './settings/GeneralTab.svelte';
  import EnginesTab from './settings/EnginesTab.svelte';
  import ShortcutsTab from './settings/ShortcutsTab.svelte';
  import HistoryTab from './settings/HistoryTab.svelte';

  type Tab = 'general' | 'engines' | 'shortcuts' | 'history';
  let tab = $state<Tab>('general');
  let curLang = $state<LangCode>(Lang.ZHCN);

  const tabs = $derived.by<{ id: Tab; label: string; icon: string }[]>(() => [
    {
      id: 'general',
      label: t('settings.general'),
      icon: 'M3 6a3 3 0 013-3h12a3 3 0 013 3v12a3 3 0 01-3 3H6a3 3 0 01-3-3V6zm4.5 3a1.5 1.5 0 100 3 1.5 1.5 0 000-3zM6 13.5a1.5 1.5 0 113 0 1.5 1.5 0 01-3 0zm4.5-6a1.5 1.5 0 100 3 1.5 1.5 0 000-3zm3 4.5a1.5 1.5 0 113 0 1.5 1.5 0 01-3 0z',
    },
    {
      id: 'engines',
      label: t('settings.engines'),
      icon: 'M10.5 2a.5.5 0 01.5.5V5h2V2.5a.5.5 0 011 0V5h2.5a.5.5 0 01.5.5v2.5h3V7a.5.5 0 011 0v2h3V8.5a.5.5 0 01.5.5v3a.5.5 0 01-.5.5h-3v2h3a.5.5 0 010 1h-3v2.5a.5.5 0 01-.5.5h-2.5v3a.5.5 0 01-1 0v-3h-2v3a.5.5 0 01-1 0v-3h-2.5a.5.5 0 01-.5-.5v-2.5h-3v3a.5.5 0 01-1 0v-3h-3a.5.5 0 01-.5-.5v-2.5h-3v3a.5.5 0 01-1 0v-3H2a.5.5 0 01-.5-.5v-3a.5.5 0 01.5-.5h3v-2H2a.5.5 0 01-.5-.5v-3a.5.5 0 01.5-.5h3V7a.5.5 0 011 0v2h3V5.5a.5.5 0 01.5-.5h2.5V2.5a.5.5 0 01.5-.5zM4 11v2H2.5v1H4v2H2.5v1H4v2.5h2.5v-2h1v2h2v-2h1v2h2.5v-2h1v2h2v-2h1v2h2.5v-2h1v2h2v-2h.5v-1H22v-2h1.5v-1H22v-2h1.5V10H22V8h-1.5v2h-1V8h-2v2h-1V8h-2v2h-1.5V8h-2v2h-1V8h-2v2h-1.5V8h-2v2h-1V8h-2v2H4zm6.5 1a1.5 1.5 0 100 3 1.5 1.5 0 000-3z',
    },
    {
      id: 'shortcuts',
      label: t('settings.shortcuts'),
      icon: 'M4 3a2 2 0 00-2 2v10a2 2 0 002 2h16a2 2 0 002-2V5a2 2 0 00-2-2H4zm0 2h16v10H4V5zm2 12v2h2v-2H6zm4 0v2h2v-2h-2zm4 0v2h2v-2h-2zm4 0v2h2v-2h-2zM6 7a1 1 0 100 2 1 1 0 000-2zm4 0a1 1 0 100 2 1 1 0 000-2zm4 0a1 1 0 100 2 1 1 0 000-2z',
    },
    {
      id: 'history',
      label: t('settings.history'),
      icon: 'M6 2a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8.828a2 2 0 00-.586-1.414l-4.828-4.828A2 2 0 0013.172 2H6zm1 2h6v5a1 1 0 001 1h5v10H7V4zm2 5a1 1 0 100 2 1 1 0 000-2zm0 4a1 1 0 100 2 1 1 0 000-2zm0 4a1 1 0 100 2 1 1 0 000-2z',
    },
  ]);

  onMount(() => {
    curLang = get(locale);
    return locale.subscribe((l) => (curLang = l));
  });
</script>

<div class="u-surface flex h-full w-full flex-col" style={rootStyleToStyle($rootStyle)}>
  <TitleBar />

  <div class="flex flex-1 overflow-hidden">
    <!-- Sidebar -->
    <nav class="u-sidebar u-border-r flex w-44 flex-col gap-1 p-4">
      <div class="u-label mb-4 px-3">
        {t('settings.title')}
      </div>
      {#each tabs as item}
        <button
          class="u-nav-item"
          class:is-active={tab === item.id}
          onclick={() => {
            tab = item.id;
          }}
        >
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <path d={item.icon} />
          </svg>
          {item.label}
        </button>
      {/each}
    </nav>

    <!-- Content -->
    <section class="flex-1 overflow-y-auto p-8">
      {#if tab === 'general'}
        <GeneralTab bind:curLang />
      {:else if tab === 'engines'}
        <EnginesTab />
      {:else if tab === 'shortcuts'}
        <ShortcutsTab />
      {:else if tab === 'history'}
        <HistoryTab />
      {/if}
    </section>
  </div>
</div>
