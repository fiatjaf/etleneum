<!-- @format -->

<script>
  import {onMount} from 'svelte'
  import {blur} from 'svelte/transition'

  import account from './accountStore'

  let names = [
    'Etleneum',
    'Этлениум',
    'Etlenums',
    '菠蘿蜜',
    'Etléneo',
    'ڪيانا',
    'Etleniko',
    'Etẽlenum',
    '素晴らしい虹',
    'Etlejëno'
  ]
  let name = 'Etleneum'
  let rotation = 0
  let visible = true
  var intv

  onMount(() => {
    intv = setInterval(() => {
      visible = false
      setTimeout(() => {
        name = names[names.indexOf(name) + (1 % (names.length - 1))]
        rotation = rotation + 60
        visible = true
      }, 1)
    }, 4000)

    return () => {
      clearInterval(intv)
    }
  })
</script>

<style>
  * {
    text-align: center;
  }
  main {
    padding: 0 100px;
  }
  header img {
    height: 14rem;
  }
  header h1 {
    font-size: 2.4rem;
    font-weight: normal;
  }
  del {
    text-decoration: line-through 4px white;
  }
</style>

<main>
  <header class="center">
    {#if visible}
    <img
      in:blur="{{amount:10, duration: 450}}"
      out:blur="{{amount: 10, duration: 450}}"
      src="/static/icon.png"
      alt="etleneum logo"
      style="transform: rotate({rotation}deg)"
    />
    <h1
      in:blur="{{amount:10, duration: 450}}"
      out:blur="{{amount: 10, duration: 450}}"
    >
      {name}
    </h1>
    {/if}
  </header>
  <h2>
    Etleneum is a global, open-source platform for <del>de</del
    ><ins>the</ins> centralized applications.
  </h2>
  <p>
    On Etleneum you can write code that controls digital value, runs exactly as
    programmed, and is accessible anywhere in the world.
  </p>

  <article>
    <h2>Etleneum in 2 minutes</h2>
    <p>
      Etleneum is not just a pun with Ethereum, it's a real smart contract
      platform. You can build publicly auditable and trusted applications that
      run custom code, can talk to other services and are accessible through an
      API, all using a built-in user account system (optional) and real
      Lightning payments.
    </p>
    <img alt="contract schema drawing" src="/static/bet.png" />
    <p>
      Above you see a graphical example of a contract with two methods:
      <b>bet</b> and <b>resolve</b>. Account <i>74</i> made a bet with account
      <i>12</i> when both called the <b>bet</b> method (details of the contract
      and calls are hidden for brevity). Then later an anonymous oracle called
      <b>resolve</b> and settled the bet. Account <i>12</i> ended up with all
      the satoshis.
    </p>
    <p>
      Contracts are just that: a set of <b>methods</b>, some <b>funds</b> and a
      JSON <b>state</b>. <b>Calls</b> can be <i>identified</i> or not, and it
      can <i>contain satoshis</i> or not. Each call <i>modifies the state</i> in
      a certain way and can also <i>transfer funds</i> from the contract to an
      account.
    </p>
  </article>

  <p>
    <a href="#/docs">Read the docs</a>{#if window.location.host ===
    'etleneum.com'}&nbsp;and start creating some <em>free</em> contracts at the
    <a href="https://test.etleneum.com">test website</a>{/if}.
  </p>

  {#if window.location.host === 'etleneum.com'}
  <p>
    If you need help, have questions about Etleneum or any of the contracts, go
    talk to us at
    <a href="https://t.me/etleneum">our Telegram chat</a>. Also follow
    <a href="https://twitter.com/etleneum2">@etleneum2</a> on Twitter.
  </p>
  {:else}
  <p>
    This is a test website. You can create contracts and run calls for free.
    Whenever you see an invoice, just wait and it will be paid automatically
    after some seconds. You can login and have a balance, but withdrawals are
    impossible.
  </p>
  {/if}
</main>
