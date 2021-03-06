<script>
  import bech32 from 'bech32'
  import {onMount} from 'svelte'
  import {replace, push} from 'svelte-spa-router'

  import QR from './QR.svelte'
  import Json from './Json.svelte'
  import LuaCode from './LuaCode.svelte'
  import Markdown from './Markdown.svelte'
  import MultiField from './MultiField.svelte'

  import account, {hmacCall} from './accountStore'
  import * as toast from './toast'

  export let params
  var contract

  var payloadfields = {}
  var nextcall
  var withamount = true
  resetNextCall()

  $: nextmethod = (() => {
    if (nextcall && contract) {
      for (let i = 0; i < contract.methods.length; i++) {
        if (contract.methods[i].name === nextcall.method) {
          return contract.methods[i]
        }
      }
    }
  })()

  onMount(loadContract)

  async function loadContract() {
    let r = await fetch('/~/contract/' + params.ctid)
    if (r.status == 404) return replace('/notfound')
    let ctdata = (await r.json()).value
    contract = ctdata

    resetNextCall()
  }

  function resetNextCall(id) {
    if (id && nextcall.id !== id) return

    nextcall = {
      msatoshi: 0,
      payload: {},
      method: null,
      includeCallerSession: false
    }
  }

  async function prepareCall(e) {
    e.preventDefault()

    let {method, msatoshi, payload, includeCallerSession} = nextcall

    if (!method) {
      toast.warning('you must select a method to call!')
      return
    }

    let qs =
      $account.session && includeCallerSession
        ? `?session=${$account.session}`
        : ''

    let r = await fetch('/~/contract/' + params.ctid + '/call' + qs, {
      method: 'post',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        method,
        msatoshi,
        payload
      })
    })
    let resp = await r.json()

    if (!resp.ok) {
      toast.warning(resp.error)
      return
    }

    let {id, invoice} = resp.value
    nextcall.invoice = invoice
    nextcall.id = id
  }

  async function executeWithBalance(e) {
    e.preventDefault()

    let {method, msatoshi, payload} = nextcall

    if (!method) {
      toast.warning('you must select a method to call!')
      return
    }

    let qs = `?session=${$account.session}&use-balance=true`

    let r = await fetch('/~/contract/' + params.ctid + '/call' + qs, {
      method: 'post',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        method,
        msatoshi,
        payload
      })
    })
    let resp = await r.json()

    if (!resp.ok) {
      toast.warning(resp.error)
      return
    }

    resetNextCall()
  }

  function abandonCall(e) {
    e.preventDefault()
    resetNextCall()
  }

  function lnurlpay(nextcall, withamount = true) {
    let amountpath = withamount ? `/${nextcall.msatoshi}` : ''

    var url = new URL(
      `/lnurl/contract/${params.ctid}/call/${nextcall.method}${amountpath}`,
      location
    )
    var qs = new URLSearchParams()
    for (let k in nextcall.payload) {
      qs.set(k, nextcall.payload[k])
    }
    if (nextcall.includeCallerSession) {
      qs.set('_account', $account.id)
      qs.set('_hmac', hmacCall(contract.id, nextcall))
    }
    url.search = qs.toString()

    return bech32.encode(
      'lnurl',
      bech32.toWords(
        url
          .toString()
          .split('')
          .map(l => l.charCodeAt(0))
      ),
      5000
    )
  }

  async function deleteContract(e) {
    e.preventDefault()

    let r = await fetch('/~/contract/' + params.ctid, {method: 'delete'})
    let resp = await r.json()
    if (resp.ok) {
      toast.success('contract deleted!')
      push('/contracts')
    } else {
      toast.warning(resp.error)
    }
  }

  function setMsatoshi(e) {
    nextcall.msatoshi = parseInt(e.target.value) * 1000
  }

  function setMethod(e) {
    e.preventDefault()
    let method = e.target.innerHTML.trim()

    nextcall.method = nextcall.method === method ? null : method

    for (let k in nextcall.payload) {
      if (k in payloadfields[method]) continue
      delete nextcall.payload[k]
    }
  }

  function setPayloadField(pf, e) {
    nextcall.payload[pf] = e.detail
  }

  function renderPayloadField(pf) {
    return nextcall.payload[pf] === undefined
      ? ''
      : typeof nextcall.payload[pf] === 'string'
      ? nextcall.payload[pf]
      : JSON.stringify(nextcall.payload[pf])
  }

  onMount(() => {
    startEventSource()

    return () => es.close()
  })

  var es

  function startEventSource() {
    es = new EventSource('/~~~/contract/' + params.ctid)
    es.onerror = e => console.log('contract sse error', e.data)
    es.addEventListener('call-run-event', e => {
      let data = JSON.parse(e.data)
      switch (data.kind) {
        case 'start':
          toast.info(`call ${data.id} started...`)
          break
        case 'print':
          toast.info(`call ${data.id} printed: ${data.message}`)
          break
        case 'function':
          toast.info(`call ${data.id} used a function: ${data.message}`)
          account.refresh()
          break
      }
    })
    es.addEventListener('call-made', e => {
      let data = JSON.parse(e.data)
      setTimeout(() => {
        toast.success(`call ${data.id} made!`)
        resetNextCall(data.id)
        loadContract()
      }, 1000)
    })
    es.addEventListener('call-error', e => {
      let data = JSON.parse(e.data)

      if (data.kind === 'internal') {
        toast.error(`internal error, please notify: ${data.message}`)
      } else if (data.kind === 'runtime') {
        toast.warning(`raised error: <pre>${data.message}</pre>`, 30000)
      } else if (data.kind === 'balance') {
        toast.warning(`balance is insufficient!`, 30000)
      }

      resetNextCall(data.id)
    })
  }
</script>

<svelte:head>
  {#if contract && contract.name}
    <title>[{contract.id}] {contract.name} | etleneum contract</title>
  {/if}
</svelte:head>

{#if !contract}
  <div class="center">loading</div>
{:else}
  <div id="main">
    <div id="status">
      <h1>
        {contract.name}
        {#if window.location.host !== 'etleneum.com'}
          <button on:click={deleteContract}>delete</button>{/if}
      </h1>
      <div>{(contract.funds / 1000).toFixed(3)} sat</div>
      <h4>state</h4>
      <Json value={contract.state} />
      <h4>readme</h4>
      <Markdown
        ctdata={{
          funds: contract.funds,
          state: contract.state,
          id: contract.id
        }}>{contract.readme}</Markdown
      >
      <h4>code</h4>
      <LuaCode>{contract.code}</LuaCode>
      <h4>
        <a
          href={`https://github.com/${process.env.GITHUB_REPO}/commits/master/contracts/${contract.id}`}
          target="_blank">contract history</a
        >
      </h4>
    </div>
    <div id="action">
      <h2>make a call</h2>
      {#if contract.methods.length == 0}
        <p>apparently this contract has no callable methods</p>
      {:else}
        {#if nextcall.invoice}
          <div class="center">
            <QR value={nextcall.invoice} />
            <p>{nextcall.id}<br />pay to make the call</p>
            <button on:click={abandonCall}>prepare a different call</button>
          </div>
        {:else}
          <form on:submit={prepareCall}>
            <div class="label">
              method: {#if nextmethod}<b>{nextmethod.name}</b>{:else}none
                selected{/if}
              <div class="select">
                {#each contract.methods as method (method.name)}
                  <button
                    on:click={setMethod}
                    class:enabled={method.name === nextcall.method}
                  >
                    {method.name}
                  </button>
                {/each}
              </div>
              {#if nextmethod && nextmethod.auth}
                <div>
                  apparently this method requires you to be authenticated
                </div>
              {/if}
            </div>
            {#if nextmethod}
              <label>
                satoshi: <input
                  type="number"
                  min="0"
                  value={nextcall.msatoshi / 1000}
                  on:input={setMsatoshi}
                />
              </label>
              {#each nextmethod.params as pf (pf)}
                <label for="_">
                  {pf}:
                  <MultiField
                    value={renderPayloadField(pf)}
                    on:change={e => setPayloadField(pf, e)}
                  />
                </label>
              {/each}
              {#if Object.keys(nextcall.payload).length > 0}
                <label for="_"
                  >payload: <Json value={nextcall.payload} />
                </label>
              {/if}
              <label class:disabled={!$account.session}>
                make this call authenticated with your account:
                <input
                  type="checkbox"
                  disabled={!$account.session}
                  bind:checked={nextcall.includeCallerSession}
                />
              </label>
              <div>
                <button>prepare call</button>
                {#if nextcall.includeCallerSession}
                  <button on:click={executeWithBalance}>
                    execute using funds from balance
                  </button>
                {/if}
              </div>
            {/if}
          </form>
        {/if}
        {#if nextmethod && !nextcall.invoice}
          <div id="lnurl-pay">
            <h3>reusable lnurl-pay for this call</h3>
            {#if nextcall.includeCallerSession}
              <p>(includes <b>secret</b> auth token)</p>
            {/if}
            <div class="center">
              <QR value={lnurlpay(nextcall, withamount)} size={200} />
            </div>
            <div>
              <label
                ><input type="checkbox" bind:checked={withamount} /> with amount</label
              >
            </div>
          </div>
        {/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  #main {
    display: flex;
    flex-wrap: wrap;
  }
  #status {
    width: 58%;
  }
  #action {
    margin-left: 2%;
    width: 40%;
  }
  #action label,
  #action .label {
    display: block;
    margin: 12px 2px;
  }
  #action .select {
    display: flex;
    flex-wrap: wrap;
  }
  #action input:not([type='checkbox']),
  #action .select button {
    padding: 4px;
    font-size: 1.2rem;
    background-color: var(--lightgrey);
  }
  #action .select button.enabled {
    background: var(--yellow);
    border: inset var(--lightgrey) 3px;
  }
  .disabled {
    color: grey;
  }
  #lnurl-pay {
    margin-top: 70px;
    text-align: center;
  }
  #lnurl-pay h3 {
    margin: 0;
  }
  button {
    cursor: pointer;
    margin: 12px;
    padding: 12px;
    font-size: 1.2rem;
    background-color: var(--yellow);
  }
</style>
