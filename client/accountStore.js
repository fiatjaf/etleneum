/** @format */

import {readable} from 'svelte/store'
import hmac from 'hmac'
import shajs from 'sha.js'

import * as toast from './toast'

function getInitial() {
  return {
    lnurl: {auth: null, withdraw: null},
    session: window.localStorage.getItem('auth-session') || null,
    id: null,
    balance: 0,
    secret: '',
    history: []
  }
}

var current = getInitial()
var es
var storeSet = () => {}

const account = readable(current, set => {
  storeSet = set
  startEventSource()

  return () => {
    es.close()
  }
})

account.refresh = function() {
  window.fetch('/~/refresh?session=' + current.session)
}

account.reset = function() {
  if (es) {
    es.close()
  }

  window.localStorage.removeItem('auth-session')
  current = getInitial()
  storeSet(current)

  startEventSource()
}

function startEventSource() {
  es = new window.EventSource(
    '/~~~/session?src=store&session=' + (current.session ? current.session : '')
  )
  es.onerror = e => console.log('accountstore sse error', e.data)
  es.addEventListener('lnurls', e => {
    let data = JSON.parse(e.data)
    current = {...current, lnurl: data}
    storeSet(current)
  })
  es.addEventListener('auth', e => {
    let data = JSON.parse(e.data)
    current = {
      ...current,
      session: data.session || current.session,
      id: data.account,
      balance: data.balance,
      secret: data.secret
    }
    storeSet(current)

    if (data.session) {
      window.localStorage.setItem('auth-session', data.session)
    }
  })
  es.addEventListener('history', e => {
    let data = JSON.parse(e.data)
    current.history = data
    storeSet(current)
  })
  es.addEventListener('withdraw', e => {
    let data = JSON.parse(e.data)
    current = {...current, balance: data.new_balance}
    storeSet(current)
  })
  es.addEventListener('error', e => {
    toast.error(e.data)
  })
}

export default account

export function hmacCall(contractId, call) {
  var res = `${contractId}:${call.method}:${call.msatoshi},`

  var keys = Object.keys(call.payload).sort()
  for (let i = 0; i < keys.length; i++) {
    let k = keys[i]
    let v = call.payload[k]
    res += `${k}=${v}`
    res += ','
  }

  return hmac(() => shajs('sha256'), 64, current.secret)
    .update(res, 'utf8')
    .digest('hex')
}
