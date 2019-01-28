open Json.Decode;
open Js.Promise;

type call = {
  id: string,
  hash: string,
  time: string,
  contract_id: string,
  method: string,
  payload: string,
  cost: int,
  satoshis: int,
};

type contract = {
  id: string,
  code: string,
  name: string,
  readme: string,
  state: Js.Json.t,
  funds: int,
  bolt11: option(string),
};

let decodeContract = json => {
  id: json |> field("id", string),
  name: json |> field("name", string),
  readme: json |> field("readme", string),
  code: json |> field("code", string),
  state: json |> field("state", x => x),
  funds: json |> field("funds", int),
  bolt11: json |> optional(field("bolt11", string)),
};

let decodeContractList = list(decodeContract);

let fetchContractsList = () =>
  Fetch.fetch("/~/contracts")
  |> then_(Fetch.Response.json)
  |> then_(json => json |> decodeContractList |> resolve);
