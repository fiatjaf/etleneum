/** @format */

import {readable} from 'svelte/store'

const initial = {
  lnurl: {auth: null, withdraw: null},
  session: window.localStorage.getItem('auth-session') || null,
  id: null,
  balance: 0
}

var current = {...initial}
var es
var storeSet = () => {}

const account = readable(initial, set => {
  storeSet = set
  es = startEventSource()

  return () => {
    es.close()
  }
})

account.reset = function() {
  window.localStorage.removeItem('auth-session')
  current = {...initial}
  storeSet(current)

  if (es) {
    es.close()
  }
  startEventSource()
}

function startEventSource() {
  es = new window.EventSource(
    '/lnurl/session?session=' + (current.session ? current.session : '')
  )

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
      balance: data.balance
    }
    storeSet(current)

    if (data.session) {
      window.localStorage.setItem('auth-session', data.session)
    }
  })
  es.addEventListener('withdraw', e => {
    let data = JSON.parse(e.data)
    current = {...current, balance: data.new_balance}
    storeSet(current)
  })
}

export default account
