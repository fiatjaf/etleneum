<!-- @format -->
<script>
  import {onMount, setContext} from 'svelte'
  import Router, {link} from 'svelte-spa-router'

  import Home from './Home.svelte'
  import List from './List.svelte'
  import View from './View.svelte'
  import Create from './Create.svelte'
  import Account from './Account.svelte'
  import Docs from './Docs.svelte'
  import NotFound from './NotFound.svelte'

  import account from './accountStore'

  const routes = {
    '/': Home,
    '/docs': Docs,
    '/contracts': List,
    '/create': Create,
    '/contract/:ctid': View,
    '/account': Account,
    '*': NotFound
  }
</script>

<nav>
  <a href="#/"> <img alt="etleneum logo" src="/static/icon.png" /></a>
  <a href="#/contracts">list contracts</a>
  <a href="#/create">create</a>
  <a href="#/account">
    {#if $account.id}acct:<span class="account">{$account.id}</span
      >{:else}login{/if}
  </a>
  <a href="#/docs">docs</a>
</nav>
<main><Router {routes} /></main>
<footer>
  <span>Etleneum.com</span>
  <a
    href="https://ln.bigsun.xyz/node/02bed1812d3824f7cc4ccd38da5d66a29fcfec146fe95e26cd2e0d3f930d653a8d"
  >
    <img
      alt="_"
      src="https://img.shields.io/badge/dynamic/json?color=orange&label=node%2002bed%E2%80%A6&query=%24%5B0%5D.openchannels&suffix=%20channels&url=https%3A%2F%2Fln.bigsun.xyz%2Fapi%2Fnodes%3Fpubkey%3Deq.02bed1812d3824f7cc4ccd38da5d66a29fcfec146fe95e26cd2e0d3f930d653a8d%26select%3Dopenchannels"
    />
  </a>
  <a
    href="https://ln.bigsun.xyz/node/02bed1812d3824f7cc4ccd38da5d66a29fcfec146fe95e26cd2e0d3f930d653a8d"
  >
    <img
      alt="_"
      src="https://img.shields.io/badge/dynamic/json?color=blue&label=software&query=%24[0].software&url=https%3A%2F%2Fln.bigsun.xyz%2Fapi%2Fnodes%3Fpubkey%3Deq.02bed1812d3824f7cc4ccd38da5d66a29fcfec146fe95e26cd2e0d3f930d653a8d%26select%3Dsoftware"
    />
  </a>
  <a href="/static/terms.txt">Terms</a>
</footer>

<style>
  nav {
    text-transform: uppercase;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  nav > * {
    margin: 0 14px;
  }
  nav a:hover {
    padding: 3px 6px;
  }
  nav img {
    height: 3rem;
  }
  nav a {
    color: inherit;
  }
  main {
    margin: 23px auto;
  }
  footer {
    margin-top: 200px;
    display: flex;
    justify-content: center;
  }
  footer > * {
    margin: 0 20px;
  }
</style>
