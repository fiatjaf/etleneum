open Js.Promise;

type contract = {
  id: string,
  code: string,
  name: string,
  readme: string,
  created_at: string,
  storage_costs: int,
  refilled: int,
  state: Js.Json.t,
  funds: int,
  bolt11: option(string),
  invoice_paid: bool,
}
and call = {
  id: string,
  time: string,
  contract_id: string,
  method: string,
  payload: Js.Json.t,
  cost: int,
  satoshis: int,
  paid: int,
  bolt11: option(string),
  invoice_paid: bool,
}
and result = {
  ok: bool,
  value: Js.Json.t,
  error: string,
};

let emptyContract = {
  id: "",
  code: "",
  name: "",
  readme: "",
  state: Json.Encode.object_([]),
  funds: 0,
  created_at: "1970-01-01",
  storage_costs: 0,
  refilled: 0,
  bolt11: None,
  invoice_paid: false,
};
let emptyCall = {
  id: "",
  time: "",
  contract_id: "",
  method: "",
  payload: Json.Encode.object_([]),
  cost: 0,
  satoshis: 0,
  paid: 0,
  bolt11: None,
  invoice_paid: false,
};

module Decode = {
  open Json.Decode;

  let result = json => {
    ok: json |> field("ok", bool),
    value: json |> field("value", x => x),
    error:
      json |> withDefault("Unidentified error.", field("error", string)),
  };

  let contract = json => {
    id: json |> (field("id", string) |> withDefault(emptyContract.id)),
    name: json |> (field("name", string) |> withDefault(emptyContract.name)),
    readme:
      json |> (field("readme", string) |> withDefault(emptyContract.readme)),
    code: json |> (field("code", string) |> withDefault(emptyContract.code)),
    state:
      json |> (field("state", x => x) |> withDefault(emptyContract.state)),
    created_at:
      json
      |> (
        field("created_at", string) |> withDefault(emptyContract.created_at)
      ),
    storage_costs:
      json
      |> (
        field("storage_costs", int)
        |> withDefault(emptyContract.storage_costs)
      ),
    refilled:
      json |> (field("refilled", int) |> withDefault(emptyContract.refilled)),
    funds: json |> (field("funds", int) |> withDefault(emptyContract.funds)),
    bolt11: json |> optional(field("invoice", string)),
    invoice_paid:
      json |> (field("invoice_paid", bool) |> withDefault(false)),
  };

  let contractResponse = json => json |> result |> (r => r.value |> contract);
  let contractListResponse = json =>
    json |> result |> (r => r.value |> list(contract));

  let call = json => {
    id: json |> (field("id", string) |> withDefault(emptyCall.id)),
    time: json |> (field("time", string) |> withDefault(emptyCall.time)),
    contract_id:
      json
      |> (field("contract_id", string) |> withDefault(emptyCall.contract_id)),
    method:
      json |> (field("method", string) |> withDefault(emptyCall.method)),
    payload:
      json |> (field("payload", x => x) |> withDefault(emptyCall.payload)),
    cost: json |> (field("cost", int) |> withDefault(emptyCall.cost)),
    satoshis:
      json |> (field("satoshis", int) |> withDefault(emptyCall.satoshis)),
    paid: json |> (field("paid", int) |> withDefault(emptyCall.paid)),
    bolt11: json |> optional(field("invoice", string)),
    invoice_paid:
      json |> (field("invoice_paid", bool) |> withDefault(false)),
  };

  let callResponse = json => json |> result |> (r => r.value |> call);
  let callListResponse = json =>
    json |> result |> (r => r.value |> list(call));
};

module Encode = {
  open Json.Encode;

  let contract = ct =>
    object_([
      ("id", string(ct.id)),
      ("code", string(ct.code)),
      ("name", string(ct.name)),
      ("readme", string(ct.readme)),
      ("state", ct.state),
      ("funds", int(ct.funds)),
    ])
    |> Js.Json.stringify;

  let call = c =>
    object_([
      ("method", string(c.method)),
      ("payload", c.payload),
      ("satoshis", int(c.satoshis)),
    ])
    |> Js.Json.stringify;
};

module Contract = {
  let list = () =>
    Fetch.fetch("/~/contracts")
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.contractListResponse |> resolve);

  let get = id =>
    Fetch.fetch("/~/contract/" ++ id)
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.contractResponse |> resolve);

  let prepare = contract =>
    Fetch.fetchWithInit(
      "/~/contract",
      Fetch.RequestInit.make(
        ~method_=Fetch.Post,
        ~body=Fetch.BodyInit.make(contract |> Encode.contract),
        ~headers=Fetch.HeadersInit.make({"Content-Type": "application/json"}),
        (),
      ),
    )
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.contractResponse |> resolve);

  let make = (id: string) =>
    Fetch.fetchWithInit(
      "/~/contract/" ++ id,
      Fetch.RequestInit.make(~method_=Fetch.Post, ()),
    )
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.result |> resolve);
};

module Call = {
  let list = contract_id =>
    Fetch.fetch("/~/contract/" ++ contract_id ++ "/calls")
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.callListResponse |> resolve);

  let get = id =>
    Fetch.fetch("/~/call/" ++ id)
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.callResponse |> resolve);

  let prepare = (contract_id: string, call: call) =>
    Fetch.fetchWithInit(
      "/~/contract/" ++ contract_id ++ "/call",
      Fetch.RequestInit.make(
        ~method_=Fetch.Post,
        ~body=Fetch.BodyInit.make(call |> Encode.call),
        ~headers=Fetch.HeadersInit.make({"Content-Type": "application/json"}),
        (),
      ),
    )
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.callResponse |> resolve);

  let make = (id: string) =>
    Fetch.fetchWithInit(
      "/~/call/" ++ id,
      Fetch.RequestInit.make(~method_=Fetch.Post, ()),
    )
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.result |> resolve);
};

module LS = {
  let getItem = k => Dom.Storage.localStorage |> Dom.Storage.getItem(k);
  let setItem = (k, v) =>
    Dom.Storage.localStorage |> Dom.Storage.setItem(k, v);
};

module Helpers = {
  let fre = Revamp.Compiled.make({|function +([^_][^ ]*) *\( *\)|});

  let parseMethods = code =>
    Js.String.split("\n", code)
    |> Array.to_list
    |> List.map(line =>
         switch (line |> Revamp.Compiled.captures(fre) |> Rebase.Seq.head) {
         | None => ""
         | Some(g) =>
           switch (g |> Rebase.List.head) {
           | None => ""
           | Some(None) => ""
           | Some(Some(x)) => x
           }
         }
       )
    |> List.filter(x => x != "");
};
