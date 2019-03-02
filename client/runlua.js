/** @format */

const fengari = require('fengari')
const flua = require('flua')
const invoice = require('lightnode-invoice')
const sha256 = require('js-sha256').sha256
const fs = require('fs')

const sandbox = fs.readFileSync('./runlua/assets/sandbox.lua', 'utf-8')

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
    lnpay: function(bolt11, filters) {
      filters = filters || {}
      let res = invoice.decode(bolt11)
      let amountsats = res.amount * 100000000
      let amountmsats = amountsats * 1000

      if (filters.max && amountsats > filters.max) {
        return [0, "max doesn't match"]
      }
      if (filters.exact && amountsats != filters.exact) {
        return [0, "exact doesn't match"]
      }
      if (filters.hash && res.paymentHash != filters.hash) {
        return [0, "hash doesn't match"]
      }
      if (filters.payee && res.payeeNode != filters.payee) {
        return [0, "payee doesn't match"]
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

custom_env = {
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
}

for k, v in pairs(custom_env) do
  sandbox_env[k] = v
end

function call ()
${code}

  return ${method}()
end

ret = run(sandbox_env, call)
    `

  try {
    console.log(fullcall)
    let {state, ret} = flua.runWithGlobals(globals, fullcall, ['state', 'ret'])
    stateAfter = state
    returnedValue = ret
  } catch (e) {
    error = e.message

    let res = /:(\d+):/.exec(e.message)
    if (res && res.length > 1) {
      let line = parseInt(res[1])
      error +=
        '\n' +
        fullcall
          .split('\n')
          .slice(line - 3, line + 3)
          .map((l, i) => `${i + 1 + line - 3} ${l}`.padStart(3))
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
