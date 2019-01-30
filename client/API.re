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
};

module Decode = {
  open Json.Decode;

  let contract = json => {
    id: json |> field("id", string),
    name: json |> field("name", string),
    readme: json |> field("readme", string),
    code: json |> field("code", string),
    state: json |> field("state", x => x),
    created_at: json |> field("created_at", string),
    funds: json |> field("funds", int),
    bolt11: json |> optional(field("bolt11", string)),
  };

  let contractList = list(contract);

  let call = json => {
    id: json |> field("id", string),
    time: json |> field("time", string),
    contract_id: json |> field("contract_id", string),
    method: json |> field("method", string),
    payload: json |> field("payload", x => x),
    cost: json |> field("cost", int),
    satoshis: json |> field("satoshis", int),
    paid: json |> field("paid", int),
  };

  let callList = list(call);

  let ok = json => json |> field("ok", bool);
};

module Encode = {
  open Json.Encode;

  let contract = ct =>
    object_([
      ("id", string(ct.id)),
      ("code", string(ct.code)),
      ("name", string(ct.name)),
      ("readme", string(ct.readme)),
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

let fetchContractsList = () =>
  Fetch.fetch("/~/contracts")
  |> then_(Fetch.Response.json)
  |> then_(json => json |> Decode.contractList |> resolve);

let fetchContract = id =>
  Fetch.fetch("/~/contract/" ++ id)
  |> then_(Fetch.Response.json)
  |> then_(json => json |> Decode.contract |> resolve);

let fetchCalls = contract_id =>
  Fetch.fetch("/~/contract/" ++ contract_id ++ "/calls")
  |> then_(Fetch.Response.json)
  |> then_(json => json |> Decode.callList |> resolve);

let prepareContract = contract =>
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

let updateContract = (id: string, contract: contract) =>
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

let initContract = (id: string, contract: contract) =>
  Fetch.fetchWithInit(
    "/~/contract/" ++ id,
    Fetch.RequestInit.make(
      ~method_=Fetch.Post,
      ~body=Fetch.BodyInit.make(contract |> Encode.contract),
      ~headers=Fetch.HeadersInit.make({"Content-Type": "application/json"}),
      (),
    ),
  )
  |> then_(Fetch.Response.json)
  |> then_(json => json |> Decode.ok |> resolve);
