let component = ReasonReact.statelessComponent("Docs");

[@bs.module "./markdown.js"] external markdown: string => string = "markdown";

let make = _children => {
  ...component,
  render: _self =>
    <div>
      <article
        dangerouslySetInnerHTML={
          "__html":
            markdown(
              "
# Writing a contract

  A contract consists of a **state**, some **funds** and multiple **methods**, which can be called by anyone and may affect the contract state, make GET requests to other places on the internet and pay Lightning Network invoices. What we call methods are just Lua functions.

  See the following example of a simple ICO contract code:

```
function __init__ ()
  return {
    name='my ico',
    balances={}
  }
end

function buytoken ()
  local price = 5
  local amount = payload.amount
  local user = payload.user

  if satoshis == amount * price then
    current = state.balances[user]
    if current == nil then
      current = 0
    end
    state.balances[user] = current + amount
  else
    error('insufficient payment')
  end
end

function cashout ()
  local invoice = payload._invoice
  ln.pay(invoice, {hash='fe713e5392f149bc15f8fc94f81b5c86a0a1cae886a7e1b7d1fa3b80827aef88'})
end
```

  All what that contract does is to accept satoshis in the `buytoken` method and assign balances of an unnamed mysterious token to the payer. Each token costs 5 satoshis and the method checks if the quantity the buyer wants matchs the amount of satoshis he has included in the call. Note that, because this is just an example, there aren't any considerations of identification or authentication, users are just arbitrary names defined by the buyer.

  The other method, `cashout` is used by the token issuer to grab the money it has secured in the ICO and go away. As the contract is just an example, the method it uses to cash out is to send an invoice with a predefined hash (to which he must know the preimage beforehand). Since a preimage can be only used once this contract will only be able to be cashed-out once.

  Other things you must know:

  * Each contract must have an `__init__` method. That is a special method that is called only when the contract is created, it must return a Lua table that will server as the initial contract state.
  * All other top level functions are methods callable from the external world.
  * Even if an HTTP request or an `ln.pay` call fail, the call will still be ok and the contract state will still be updated and so on. If you want the call to fail completely you must call the Lua function `error()`.
  * Failed calls should be refundable, but that's not implemented yet.
  * No one is able to change a contract's code after it has been activated, not even the contract creator.

# Calling a contract

  When you make a call, you send 3 things to the contract:

  * A string **method** with the name of the contract method you're calling.
  * A JSON **payload**. It can be anything and will be available as the global
    variable `payload` inside the call.
  * A integer representing the number of **satoshis** to include in your call. Some
    methods may require you to include a certain number of satoshis so they can be
    effective. The invoice you're required to pay to make any call includes this
    number of satoshis plus a small cost. The number of satoshis is available to the
    call as the global variable `satoshis`. Regardless of what the contract code does
    with it, the satoshis are added to the contract funds.

  After sending these you'll receive an invoice. Pay that invoice and proceed to make the call. It will run the contract's given **method** and update it accordingly. That's all.

# Contract API

  Contract code have access to the standard [Lua](https://www.lua.org/manual/5.3/manual.html#6) library, excluding potentially harmful stuff. Plus the following special functions:

  * `ln.pay(invoice[, filters])` triggers a Lightning payment from the contract to the given `invoice`. `filters` is a table that can contain any combination of `payee`, `hash`, `exact` and `max`. These serve as constraints to invoices that can be paid in that call (`payee` is a Lightning node id, `hash` is the invoice payment hash, `exact` and `max` are integer amounts in satoshis). Returns `msatoshis, nil` if everything is ok or `0, errormessage` if the constraints don't match.
  * `util.sha256(string)` returns the hash of the given string.
  * `http.gettext(url[, headers])` returns the text body of the response to the given URL call or `nil, errormessage`.
  * `http.getjson(url[, headers])` does the same, but returns a table with the decoded JSON instead of raw text.
  * `os.date`, `os.time` are `os.difftime` are the same as [explained here](https://www.lua.org/pil/22.1.html)

## Hidden payload fields

  Payload fields (if you're sending a JSON object) starting with `_` will not be saved in the contract call log, so you can use them to perform things like authentication or to send invoices to be paid.

## When payment fails

  When a contract successfully calls `ln.pay`, the Lightning payment isn't send immediately, it is queued and sent afterwards. If a payment fails, for example, due to routing issues, it can be retried later with either the same invoice or a new one with the same amount (see `/~/retry` API endpoint below).

  Anyone knowing the previous invoice can replace it with a new one if it fails, so it's recommended to use hidden payload fields for the invoices.

# JSON API

  Anything you can do on this website you can also do through Etleneum's public JSON API.

  * `Contract`: `{id: String, code: String, name: String, readme: String, funds: Int}`
  * `Call`: `{id: String, time: String, method: String, payload: Any, satoshis: Int, cost: Int, paid: Int}`

  All paths start at `https://etleneum.com` and must be called with `Content-Type: application/json`. All methods are [CORS](https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS)-enabled and no authorization mechanism is required or supported.

  All calls return an object of type `{ok: Bool, error: String, value: Any}`. The relevant data is always in the `value` key and `error` is only present when the call has failed. In the following endpoint descriptions we omit the `ok/value` envelope and show just what should be inside `value`.

  * `GET` `/~/contracts` lists all the contracts, sorted by the most recent activity, returns `[Contract]`;
  * `POST` `/~/contract` prepares a new contract, takes `{name: String, code: String, readme: String}`, returns `{id: String, invoice: String}`;
  * `POST` `/~/contract/<id>` activates a previously prepared contract with a paid invoice, returns `true`;
  * `GET` `/~/contract/<id>` returns the full contract info, `Contract`;
  * `GET` `/~/contract/<id>/state` returns just the contract state, `Any`;
  * `GET` `/~/contract/<id>/funds` returns just the contract funds, in millisatoshis, `Int`;
  * `GET` `/~/contract/<id>/calls` lists all contract calls, sorted by most recent first, returns `[Call]`;
  * `POST` `/~/contract/<id>/call` prepares a new call, takes `{method: String, payload: Any, satoshis: Int}`, returns `{id: String, invoice: String}`;
  * `POST` `/~/call/<id>` makes a previously prepared call with a paid invoice, returns `Any` (the value returned by the contract, if any);
  * `GET` `/~/call/<id>` returns the full call info, `Call`;
  * `POST` `/~/refill/<contract-id>/<satoshis>` arbitrarily add funds to a contract, returns `{invoice: String}` (payments are acknowledged automatically);
  * `POST` `/~/retry/<invoice>` retries a failed payment from an `ln.pay` call on any contract, takes `{invoice: String}` or nothing, if you want to retry the same invoice.
    ",
            ),
        }
      />
    </div>,
};
