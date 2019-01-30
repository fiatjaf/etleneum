/** @format */

const glua = require('glua')

module.exports.runlua = function runlua(
  code,
  stateBefore,
  method,
  payload,
  satoshis
) {
  var stateAfter
  var returnedValue
  var paymentsDone = []

  glua.runWithGlobals(
    {
      getstateafter: function(x) {
        stateAfter = x
      },
      getreturnedvalue: function(x) {
        returnedValue = x
      },
      lnpay: function(bolt11, filters) {
        paymentsDone.push(bolt11)
        console.log(`paying ${bolt11} with filters ${filters}.`)
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

  return {
    stateAfter: stateAfter,
    returnedValue: returnedValue,
    paymentsDone: paymentsDone
  }
}
