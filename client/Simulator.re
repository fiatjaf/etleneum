type state = {
  contract: API.contract,
  temp_contract_state: option(string),
  nextcall: API.call,
  temp_call_payload: option(string),
  result: option(luaresult),
}
and luaresult = {
  state: Js.Json.t,
  ret: Js.Json.t,
  payments_done: list(string),
};

type action =
  | EditContractCode(string)
  | EditContractState(string)
  | ParseContractStateJSON
  | EditContractFunds(int)
  | EditCallMethod(string)
  | EditCallPayload(string)
  | ParseCallPayloadJSON
  | EditCallSatoshis(int)
  | SetState(state)
  | SimulateCall;

[@bs.module "./glua-simulate-environment.js"]
external runlua: (string, Js.Json.t, string, Js.Json.t, int) => Js.Json.t =
  "runlua";

let getluaresult = (json: Js.Json.t): luaresult =>
  Json.Decode.{
    state: json |> field("stateAfter", x => x),
    ret: json |> field("returnedValue", x => x),
    payments_done: json |> field("paymentsDone", list(string)),
  };

let component = ReasonReact.reducerComponent("Simulator");

let make = _children => {
  ...component,
  initialState: () => {
    contract: API.emptyContract,
    nextcall: API.emptyCall,
    temp_call_payload: None,
    temp_contract_state: None,
    result: None,
  },
  reducer: (action: action, state: state) => {
    let contract = state.contract;
    let nextcall = state.nextcall;

    switch (action) {
    | EditContractCode(code) =>
      ReasonReact.Update({
        ...state,
        contract: {
          ...contract,
          code,
        },
      })
    | EditContractState(statestr) =>
      ReasonReact.Update({...state, temp_contract_state: Some(statestr)})
    | ParseContractStateJSON =>
      ReasonReact.SideEffects(
        (
          self =>
            switch (self.state.temp_contract_state) {
            | None => ()
            | Some(temp) =>
              switch (Js.Json.parseExn(temp)) {
              | json =>
                {
                  ...state,
                  contract: {
                    ...contract,
                    state: json,
                  },
                  temp_contract_state: None,
                }
                ->SetState
                |> self.send
              | exception (Js.Exn.Error(_)) => ()
              }
            }
        ),
      )
    | EditContractFunds(funds) =>
      ReasonReact.Update({
        ...state,
        contract: {
          ...contract,
          funds,
        },
      })
    | EditCallMethod(method) =>
      ReasonReact.Update({
        ...state,
        nextcall: {
          ...nextcall,
          method,
        },
      })
    | EditCallPayload(payload) =>
      ReasonReact.Update({...state, temp_call_payload: Some(payload)})
    | ParseCallPayloadJSON =>
      ReasonReact.SideEffects(
        (
          self =>
            switch (self.state.temp_call_payload) {
            | None => ()
            | Some(temp) =>
              switch (Js.Json.parseExn(temp)) {
              | json =>
                {
                  ...state,
                  nextcall: {
                    ...nextcall,
                    payload: json,
                  },
                  temp_call_payload: None,
                }
                ->SetState
                |> self.send
              | exception (Js.Exn.Error(_)) => ()
              }
            }
        ),
      )
    | EditCallSatoshis(satoshis) =>
      ReasonReact.Update({
        ...state,
        nextcall: {
          ...nextcall,
          satoshis,
        },
      })
    | SetState(state) => ReasonReact.Update(state)
    | SimulateCall =>
      ReasonReact.Update({
        ...state,
        result:
          runlua(
            state.contract.code,
            state.contract.state,
            state.nextcall.method,
            state.nextcall.payload,
            state.nextcall.satoshis,
          )
          ->getluaresult
          ->Some,
      })
    };
  },
  render: self =>
    <div className="simulator">
      <div> <h1> {ReasonReact.string("Simulator")} </h1> </div>
      <div className="elements">
        <div className="code">
          {ReasonReact.string("Lua code: ")}
          <div>
            <textarea
              onChange={
                self.handle((event, _self) =>
                  self.send(
                    EditContractCode(event->ReactEvent.Form.target##value),
                  )
                )
              }
              onBlur={
                self.handle((_event, _self) =>
                  self.send(ParseContractStateJSON)
                )
              }
              value={self.state.contract.code}
            />
          </div>
        </div>
        <div className="state">
          <div> {ReasonReact.string("Contract state:")} </div>
          <textarea
            onChange={
              self.handle((event, _self) =>
                self.send(
                  EditContractState(event->ReactEvent.Form.target##value),
                )
              )
            }
            value={
              switch (self.state.temp_contract_state) {
              | None =>
                Js.Json.stringifyWithSpace(self.state.contract.state, 2)
              | Some(str) => str
              }
            }
          />
        </div>
      </div>
      <div className="elements">
        <div className="nextcall">
          <div> <h3> {ReasonReact.string("Next call:")} </h3> </div>
          <div>
            <h5> {ReasonReact.string("Method: ")} </h5>
            <input
              value={self.state.nextcall.method}
              onChange={
                self.handle((event, _self) =>
                  self.send(
                    EditCallMethod(event->ReactEvent.Form.target##value),
                  )
                )
              }
            />
          </div>
          <div>
            <h5> {ReasonReact.string("Satoshis: ")} </h5>
            <input
              type_="number"
              step=1.0
              min=0
              value={string_of_int(self.state.nextcall.satoshis)}
              onChange={
                self.handle((event, _self) =>
                  self.send(
                    EditCallSatoshis(
                      event->ReactEvent.Form.target##value |> int_of_string,
                    ),
                  )
                )
              }
            />
          </div>
          <div>
            <h5> {ReasonReact.string("Payload: ")} </h5>
            <textarea
              onChange={
                self.handle((event, _self) =>
                  self.send(
                    EditCallPayload(event->ReactEvent.Form.target##value),
                  )
                )
              }
              onBlur={
                self.handle((_event, _self) =>
                  self.send(ParseCallPayloadJSON)
                )
              }
              value={
                switch (self.state.temp_call_payload) {
                | None =>
                  Js.Json.stringifyWithSpace(self.state.nextcall.payload, 2)
                | Some(str) => str
                }
              }
            />
          </div>
          <div className="button">
            <button
              onClick={
                self.handle((_event, _self) => self.send(SimulateCall))
              }>
              {ReasonReact.string("Simulate call")}
            </button>
          </div>
        </div>
        <div>
          <div> <h3> {ReasonReact.string("Result:")} </h3> </div>
          {
            switch (self.state.result) {
            | None => <div />
            | Some(result) =>
              <div className="result">
                <div>
                  {ReasonReact.string("State: ")}
                  <div>
                    {
                      ReasonReact.string(
                        result.state->Js.Json.stringifyWithSpace(2),
                      )
                    }
                  </div>
                </div>
                <div>
                  {ReasonReact.string("Returned value: ")}
                  <div>
                    {
                      ReasonReact.string(
                        result.ret->Js.Json.stringifyWithSpace(2),
                      )
                    }
                  </div>
                </div>
                <div>
                  {ReasonReact.string("Payments made: ")}
                  <div>
                    {
                      ReasonReact.array(
                        Array.of_list(
                          result.payments_done
                          |> List.map(bolt11 =>
                               <div> {ReasonReact.string(bolt11)} </div>
                             ),
                        ),
                      )
                    }
                  </div>
                </div>
              </div>
            }
          }
        </div>
      </div>
    </div>,
};
