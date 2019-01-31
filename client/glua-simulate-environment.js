/** @format */

const glua = window.glua
const invoice = require('lightnode-invoice')

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
          let satoshis = res.amount * 100000000

          if (filters.max && satoshis > filters.max) {
            return 0
          }
          if (filters.exact && satoshis != filters.exact) {
            return 0
          }
          if (filters.hash && res.paymentHash != filters.hash) {
            return 0
          }

          paymentsDone.push(bolt11)
          totalPaid += satoshis

          return satoshis
        },
        state: stateBefore,
        satoshis: satoshis,
        payload: payload
      },
      'local ln = {pay=lnpay}\n\n' +
        code +
        '\n\ngetreturnedvalue(' +
        method +
        '())\n\ngetstateafter(state)'
    )
  } catch (e) {
    error = e.message
  }

  return {
    stateAfter: stateAfter,
    returnedValue: returnedValue,
    paymentsDone: paymentsDone,
    totalPaid: totalPaid,
    error: error
  }
}
