open Js.Promise;

type contract = {
  id: string,
  code: string,
  name: string,
  readme: string,
  state: Js.Json.t,
  funds: int,
  bolt11: option(string),
}
and call = {
  id: string,
  hash: string,
  time: string,
  contract_id: string,
  method: string,
  payload: string,
  cost: int,
  satoshis: int,
};

let emptyContract = {
  id: "",
  code: "",
  name: "",
  readme: "",
  state: Json.Encode.null,
  funds: 0,
  bolt11: None,
};

module Decode = {
  open Json.Decode;

  let contract = json => {
    id: json |> field("id", string),
    name: json |> field("name", string),
    readme: json |> field("readme", string),
    code: json |> field("code", string),
    state: json |> field("state", x => x),
    funds: json |> field("funds", int),
    bolt11: json |> optional(field("bolt11", string)),
  };

  let contractList = list(contract);

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
};

let fetchContractsList = () =>
  Fetch.fetch("/~/contracts")
  |> then_(Fetch.Response.json)
  |> then_(json => json |> Decode.contractList |> resolve);

let fetchContract = id =>
  Fetch.fetch("/~/contract/" ++ id)
  |> then_(Fetch.Response.json)
  |> then_(json => json |> Decode.contract |> resolve);

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
