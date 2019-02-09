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

  try {
    glua.runWithGlobals(
      {
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

          paymentsDone.push(bolt11)
          totalPaid += amountmsats

          return amountmsats
        },
        gettext: function(url, headers) {
          console.log(
            `here we would do an http get to ${url} with headers ${JSON.stringify(
              headers
            )}.`
          )
          return ''
        },
        getjson: function(url, headers) {
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
      },
      `
${sandbox}
${code}

local ln = {pay=lnpay}
local http = {gettext=gettext, getjson=getjson}

local ret = sandbox.run(${method}, {
  quota=50, env={
    print=print,
    sha256=sha256,
    ln=ln,
    http=http,
    payload=payload,
    state=state,
    satoshis=satoshis
  }
})

getstateafter(state)
getreturnedvalue(ret)
    `
    )
  } catch (e) {
    error = e.message
  }

  if (method === '__init__') {
    stateAfter = returnedValue
  }

  return {
    stateAfter: stateAfter,
    returnedValue: returnedValue,
    paymentsDone: paymentsDone,
    totalPaid: totalPaid,
    error: error
  }
}
