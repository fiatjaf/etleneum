/** @format */

const glua = window.glua
const invoice = require('lightnode-invoice')
const sha256 = require('js-sha256').sha256
const fs = require('fs')

const sandbox = fs.readFileSync('static/sandbox.lua', 'utf-8')

module.exports.runlua = function runlua(
  code,
  stateBefore,
  method,
  payload,
  satoshis
) {
  var stateAfter = null
  var returnedValue = null
  var paymentsDone = []
  var totalPaid = 0
  var error = ''

  let globals = {
    getstateafter: function(x) {
      stateAfter = x
    },
    getreturnedvalue: function(x) {
      returnedValue = x
    },
    lnpay: function(bolt11, filters) {
      filters = filters || {}
      let res = invoice.decode(bolt11)
      let amountsats = res.amount * 100000000
      let amountmsats = amountsats * 1000

      if (filters.max && amountsats > filters.max) {
        return 0
      }
      if (filters.exact && amountsats != filters.exact) {
        return 0
      }
      if (filters.hash && res.paymentHash != filters.hash) {
        return 0
      }
      if (filters.payee && res.payeeNode != filters.payee) {
        return 0
      }

      paymentsDone.push(bolt11)
      totalPaid += amountmsats

      return amountmsats
    },
    httpgettext: function(url, headers) {
      console.log(
        `here we would do an http get to ${url} with headers ${JSON.stringify(
          headers
        )}.`
      )
      return ''
    },
    httpgetjson: function(url, headers) {
      console.log(
        `here we would do an http get to ${url} with headers ${JSON.stringify(
          headers
        )}.`
      )
      return {}
    },
    print: function(arg) {
      console.log('printed from contract: ', arg)
    },
    sha256: function(preimage) {
      return sha256(preimage)
    },
    state: stateBefore,
    satoshis: satoshis,
    payload: payload
  }

  let fullcall = `
${sandbox}

require("os")

function call ()
${code}

  return ${method}()
end

local ret = sandbox.run(call, {quota=50, env={
  print=print,
  http={
    gettext=httpgettext,
    getjson=httpgetjson
  },
  util={
    sha256=sha256
  },
  ln={pay=lnpay},
  payload=payload,
  state=state,
  satoshis=satoshis
}})

getstateafter(state)
getreturnedvalue(ret)
    `

  try {
    glua.runWithGlobals(globals, fullcall)
  } catch (e) {
    error = e.message

    let res = /line:(\d+)/.exec(e.message)
    if (res && res.length > 1) {
      let line = parseInt(res[1]) - 1
      error +=
        '\n' +
        fullcall
          .split('\n')
          .slice(line - 3, line + 3)
          .map((l, i) => `${i + line - 3}`.padStart(3) + l)
          .join('\n')
    }
  }

  if (method === '__init__') {
    stateAfter = returnedValue
  }

  return {
    stateAfter: stateAfter,
    returnedValue: returnedValue,
    paymentsDone: paymentsDone,
    totalPaid: parseInt(totalPaid),
    error: error
  }
}
