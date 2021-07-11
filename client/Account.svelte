<!-- @format -->
<script>
  import {onMount} from 'svelte'
  import PromiseWindow from 'promise-window'

  import * as toast from './toast'
  import QR from './QR.svelte'
  import account from './accountStore'

  var es

  const SEEDAUTH = 'https://seed-auth.etleneum.com'
  var awaitingSeedAuth = false
  var popupBlocked = false

  onMount(() => {
    let unsubscribe = account.subscribe(({session}) => {
      if (session && !es) {
        es = new EventSource('/~~~/session?src=comp&session=' + session)
        es.onerror = e => console.log('Account component sse error', e.data)
        es.addEventListener('withdraw', e => {
          let data = JSON.parse(e.data)
          toast.info(`successfully withdrawn ${data.amount} msatoshi`)
          toast.info(`balance is now ${data.new_balance} msatoshi`)
        })
      }
    })

    return () => {
      if (es) {
        es.close()
      }
      unsubscribe()
    }
  })

  async function loginSeedAuth(e) {
    if (popupBlocked) {
      return
    } else {
      e.preventDefault()
    }

    awaitingSeedAuth = true
    try {
      await PromiseWindow.open(`${SEEDAUTH}/#/lnurl/${$account.lnurl.auth}`, {
        windowName: 'Login to etleneum.com',
        height: 500,
        width: 400
      })
    } catch (err) {
      if (err !== 'closed') {
        if (err === 'blocked') {
          popupBlocked = true
        }
        toast.warning(`${err}`)
        console.log(err)
      }
    }

    awaitingSeedAuth = false
  }

  async function logout() {
    account.reset()
    await fetch('/lnurl/logout')
    toast.info('logged out!')
  }
</script>

<div class="center">
  {#if $account.id}
    <div style="display: flex; justify-content: space-between;">
      <div>
        Logged as <b class="account">{$account.id}</b>
      </div>
      <div style="margin-left: 20px">
        <a
          href={`https://github.com/etleneum/database/commits/master/accounts/${$account.id}`}
          target="_blank">account history</a
        >
      </div>
    </div>
    <div
      style="display: flex; margin-top: 20px; justify-content: space-between;"
    >
      <p>
        Actual balance <b>{($account.balance / 1000).toFixed(3)}</b> satoshi.
      </p>
      <p>
        Can withdraw
        <b>{($account.can_withdraw / 1000).toFixed(3)}</b> satoshi.
      </p>
      <p id="balance-notice" style="flex-shrink: 2">
        The withdraw amount is your balance subtracted of an amount of
        <em>0.7%</em> reserved to pay for the Lightning withdraw costs. The
        actual fee (probably less than that) will be applied once the withdraw
        is completed so you'll have more money. Besides that a <em>0.1%</em> platform
        fee will also be applied.
      </p>
    </div>
    {#if $account.balance > 0 && $account.lnurl.withdraw}
      <QR value={$account.lnurl.withdraw} />
      <p>Scan to withdraw.</p>
    {/if}
    <div><button on:click={logout}>logout</button></div>
  {:else if awaitingSeedAuth}
    <div class="awaiting-seed-auth">
      <img alt="awaiting/loading animation" src="/static/rings.svg" />
    </div>
    Waiting for login on popup
  {:else if $account.lnurl.auth}
    <h2>lnurl login</h2>
    <QR value={$account.lnurl.auth} />
    <p>
      Scan/click with
      <a target="_blank" href="https://lightning-wallet.com/">BLW</a> or
      scan/copy-paste to
      <a target="_blank" href="https://t.me/lntxbot">@lntxbot</a> to login.
    </p>
    <p>
      Or
      <a
        on:click={loginSeedAuth}
        href="{SEEDAUTH}/#/lnurl/{$account.lnurl.auth}"
        target="_blank">login with username and password</a
      >.
    </p>
  {/if}
</div>

<style>
  button {
    cursor: pointer;
    margin: 12px;
    padding: 12px;
    background-color: var(--yellow);
    float: right;
  }
  .awaiting-seed-auth {
    width: 500px;
    height: 500px;
    display: flex;
    justify-content: center;
    align-items: center;
  }
  .awaiting-seed-auth img {
    width: 40%;
  }
  #balance-notice {
    font-size: 10px;
    margin: 0 auto 25px;
    max-width: 420px;
    text-align: justify;
  }
</style>
