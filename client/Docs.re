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

Each contract starts with

A contract consists of

# Calling a contract

# JSON API

All paths start at `https://etleneum.com` and must be called with `Content-Type: application/json`. All methods are CORS-enabled and no authorization mechanism is required or supported: everything is public.

All calls return an object of type `{ok: Bool, error: String, value: Any}`. The relevant data is always in the `value` key and `error` is only present when the call has failed. In the following endpoint descriptions we omit the `ok/value` envelope and show just the returned value that should be inside `value`.

  * `GET` `/~/contracts` lists all the contracts, returns `[Contract]`;
  * `POST` `/~/contract` prepares a new contract, takes `{name: String, code: String, readme: String}`, returns `{id: String, invoice: String}`;
  * `POST` `/~/contract/<id>` activates a previously prepared contract with a paid invoice, returns `true`;
  * `GET` `/~/contract/<id>` returns the full contract info, `Contract`;
  * `GET` `/~/contract/<id>/state` returns just the contract state, `Any`;
  * `GET` `/~/contract/<id>/funds` returns just the contract funds, in millisatoshis, `Int`;
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
