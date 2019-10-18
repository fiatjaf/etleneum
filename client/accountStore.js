/** @format */

import {readable} from 'svelte/store'

const initial = {
  lnurl: {auth: null, withdraw: null},
  session: null,
  id: null,
  balance: 0
}

var current = {...initial}

const account = readable(initial, set => {
  let es = new window.EventSource('/lnurl/session')
  es.addEventListener('lnurls', e => {
    let data = JSON.parse(e.data)
    current = {...current, lnurl: data}
    set(current)
  })
  es.addEventListener('auth', e => {
    let data = JSON.parse(e.data)
    current = {
      ...current,
      session: data.session,
      id: data.account,
      balance: data.balance
    }
    set(current)
  })
  es.addEventListener('withdraw', e => {
    let data = JSON.parse(e.data)
    current = {...current, balance: data.new_balance}
    set(current)
  })

  return () => {
    es.close()
  }
})

export default account
