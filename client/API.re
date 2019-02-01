open Js.Promise;

type contract = {
  id: string,
  code: string,
  name: string,
  readme: string,
  created_at: string,
  state: Js.Json.t,
  funds: int,
  bolt11: option(string),
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
  bolt11: None,
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
};

module Decode = {
  open Json.Decode;

  let contract = json => {
    id: json |> withDefault(emptyContract.id, field("id", string)),
    name: json |> withDefault(emptyContract.name, field("name", string)),
    readme:
      json |> withDefault(emptyContract.readme, field("readme", string)),
    code: json |> withDefault(emptyContract.code, field("code", string)),
    state: json |> withDefault(emptyContract.state, field("state", x => x)),
    created_at:
      json
      |> withDefault(emptyContract.created_at, field("created_at", string)),
    funds: json |> withDefault(emptyContract.funds, field("funds", int)),
    bolt11: json |> optional(field("invoice", string)),
  };

  let contractList = list(contract);

  let call = json => {
    id: json |> withDefault(emptyCall.id, field("id", string)),
    time: json |> withDefault(emptyCall.time, field("time", string)),
    contract_id:
      json
      |> withDefault(emptyCall.contract_id, field("contract_id", string)),
    method: json |> withDefault(emptyCall.method, field("method", string)),
    payload: json |> withDefault(emptyCall.payload, field("payload", x => x)),
    cost: json |> withDefault(emptyCall.cost, field("cost", int)),
    satoshis:
      json |> withDefault(emptyCall.satoshis, field("satoshis", int)),
    paid: json |> withDefault(emptyCall.paid, field("paid", int)),
    bolt11: json |> optional(field("invoice", string)),
  };

  let callList = list(call);

  let result = json => {
    ok: json |> field("ok", bool),
    value: json |> withDefault(Json.Encode.null, field("value", x => x)),
    error:
      json |> withDefault("Unidentified error.", field("error", string)),
  };
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
    |> then_(json => json |> Decode.contractList |> resolve);

  let get = id =>
    Fetch.fetch("/~/contract/" ++ id)
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.contract |> resolve);

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
    |> then_(json => json |> Decode.contract |> resolve);

  let update = (id: string, contract: contract) =>
    Fetch.fetchWithInit(
      "/~/contract/" ++ id,
      Fetch.RequestInit.make(
        ~method_=Fetch.Put,
        ~body=Fetch.BodyInit.make(contract |> Encode.contract),
        ~headers=Fetch.HeadersInit.make({"Content-Type": "application/json"}),
        (),
      ),
    )
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.contract |> resolve);

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
    |> then_(json => json |> Decode.callList |> resolve);

  let get = id =>
    Fetch.fetch("/~/call/" ++ id)
    |> then_(Fetch.Response.json)
    |> then_(json => json |> Decode.call |> resolve);

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
    |> then_(json => json |> Decode.call |> resolve);

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
